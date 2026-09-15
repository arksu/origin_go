"""Run main() through mcp_client.py in the live Blender session.

Builds an isolated review scene; never changes the source scene or supplied GLB.
The animation donor is merged into the runtime GLB by merge_axe_animations.py.
"""
from pathlib import Path
import json
import math
import bpy
import bmesh
from mathutils import Matrix, Quaternion, Vector

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'art_source/equipment/stone_axe'
SOURCE = Path('/Users/park/Downloads/Meshy_AI_Stone_Axe_0915172926_texture.glb')
RIG_SOURCE = ROOT / 'art_source/characters/male_commoner_v4/commoner_rigged.blend'
RUNTIME = ROOT / 'web_new/public/assets/game/equipment/stone_axe/stone_axe.glb'


def activate(obj):
    bpy.ops.object.select_all(action='DESELECT')
    obj.hide_set(False)
    obj.select_set(True)
    bpy.context.view_layer.objects.active = obj


def build_axe():
    before = set(bpy.data.objects)
    bpy.ops.import_scene.gltf(filepath=str(SOURCE))
    meshes = [obj for obj in set(bpy.data.objects) - before if obj.type == 'MESH']
    if len(meshes) != 1:
        raise RuntimeError('Expected one source axe mesh')
    axe = meshes[0]
    activate(axe)
    bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
    axe.name = 'stone_axe'
    source_triangles = sum(len(face.vertices) - 2 for face in axe.data.polygons)
    # Grip center on the lower straight part of the handle, not the mesh bounds.
    grip = Vector((.195, .00187, -.75))
    scale = .66 / axe.dimensions.z
    for vertex in axe.data.vertices:
        vertex.co = (vertex.co - grip) * scale
    editable = bmesh.new()
    editable.from_mesh(axe.data)
    bmesh.ops.remove_doubles(editable, verts=list(editable.verts), dist=.000003)
    editable.to_mesh(axe.data)
    editable.free()
    modifier = axe.modifiers.new('1200 triangle game budget', 'DECIMATE')
    modifier.ratio = 1200 / source_triangles
    modifier.use_collapse_triangulate = True
    bpy.ops.object.modifier_apply(modifier=modifier.name)
    for face in axe.data.polygons:
        face.use_smooth = False
    for material in axe.data.materials:
        material.name = 'Stone axe painted'
        shader = material.node_tree.nodes.get('Principled BSDF')
        for socket in ('Metallic', 'Roughness', 'Normal'):
            for link in list(shader.inputs[socket].links):
                material.node_tree.links.remove(link)
        shader.inputs['Metallic'].default_value = 0
        shader.inputs['Roughness'].default_value = .9
        for node in list(material.node_tree.nodes):
            if node.type != 'TEX_IMAGE':
                continue
            if any(link.to_socket == shader.inputs['Base Color'] for link in node.outputs['Color'].links):
                node.image.scale(512, 512)
                node.image.pack()
            else:
                material.node_tree.nodes.remove(node)
    RUNTIME.parent.mkdir(parents=True, exist_ok=True)
    activate(axe)
    bpy.ops.export_scene.gltf(filepath=str(RUNTIME), export_format='GLB', use_selection=True, use_active_scene=True,
                              export_animations=False, export_extras=True, export_yup=True)
    return axe, source_triangles


def load_character(scene):
    if Path(bpy.data.filepath) == RIG_SOURCE:
        source_objects = [bpy.data.objects[name] for name in ['commoner_rig', 'meshy_commoner', 'meshy_commoner_low']]
        rig, mesh, low = [obj.copy() for obj in source_objects]
        rig.data = rig.data.copy()
        for obj in (mesh, low):
            obj.parent = rig
            for modifier in obj.modifiers:
                if modifier.type == 'ARMATURE':
                    modifier.object = rig
    else:
        with bpy.data.libraries.load(str(RIG_SOURCE), link=False) as (source, target):
            target.objects = ['commoner_rig', 'meshy_commoner', 'meshy_commoner_low']
        rig, mesh, low = target.objects
    for obj in (rig, mesh, low):
        scene.collection.objects.link(obj)
    for track in rig.animation_data.nla_tracks:
        track.mute = True
    low.hide_render = True
    low.hide_set(True)
    return rig, mesh, low


def author_arms(rig, scene):
    base_actions = {track.name: track.strips[0].action for track in rig.animation_data.nla_tracks}
    rest = {bone.name: bone.matrix_local.copy() for bone in rig.data.bones}
    authored = []
    for side, sign in [('l', 1), ('r', -1)]:
        for movement in ['idle', 'walk']:
            samples = []
            rig.animation_data.action = base_actions[movement]
            for frame in range(1, 50):
                scene.frame_set(frame)
                samples.append({bone.name: bone.matrix.copy() for bone in rig.pose.bones})
            name = f'axe_{movement}_{side}'
            action = bpy.data.actions.new(name)
            rig.animation_data.action = action
            for frame, base in enumerate(samples, 1):
                desired = dict(base)
                upper_key, fore_key, hand_key = [f'{joint}.{side}' for joint in ('upper_arm', 'forearm', 'hand')]
                # Retain the actual gait's timing but reduce its shoulder swing.
                original = base[upper_key].to_3x3().col[1]
                swing = math.atan2(original.y, -original.z) * .28 if movement == 'walk' else 0
                swing_rotation = Matrix.Rotation(swing, 3, 'X')
                shoulder = base[upper_key].translation
                upper_direction = swing_rotation @ Vector((.075 * sign, -.02, -.226)).normalized()
                fore_direction = swing_rotation @ Vector((.045 * sign, -.105, -.20)).normalized()
                location = shoulder
                for key, direction in [(upper_key, upper_direction), (fore_key, fore_direction), (hand_key, fore_direction)]:
                    bone = rig.data.bones[key]
                    rotation = rest[key].to_3x3().col[1].rotation_difference(direction) @ rest[key].to_quaternion()
                    desired[key] = Matrix.LocRotScale(location, rotation, Vector((1, 1, 1)))
                    location = location + direction * bone.length
                # Only arm channels: torso/legs/opposite hand remain base locomotion.
                for key in (upper_key, fore_key, hand_key):
                    bone = rig.data.bones[key]
                    basis = rest[key].inverted() @ rest[bone.parent.name] @ desired[bone.parent.name].inverted() @ desired[key]
                    pose = rig.pose.bones[key]
                    pose.rotation_mode = 'QUATERNION'
                    pose.matrix_basis = basis
                    for channel in ('location', 'rotation_quaternion', 'scale'):
                        pose.keyframe_insert(channel, frame=frame, group=key)
            authored.append((name, action))
    rig.animation_data.action = None
    for name, action in authored:
        track = rig.animation_data.nla_tracks.new()
        track.name = name
        track.strips.new(name, 1, action)
        track.mute = True
    return authored, base_actions


def bindings(rig):
    result = {}
    for side in ('l', 'r'):
        rest = rig.data.bones[f'hand.{side}'].matrix_local
        # Head points down in a relaxed travel grip. Blade points forward,
        # away from the thigh. Local origin is inside the closed fist.
        center = Vector((.385 if side == 'l' else -.385, -.024, 1.005))
        rotation = Matrix.Rotation(math.pi / 2, 4, 'Z') @ Matrix.Rotation(math.pi, 4, 'X')
        world = Matrix.Translation(center) @ rotation
        local = rest.inverted() @ world
        # glTF applies Y-up conversion to scene roots, not bone-local axes.
        # The separate axe asset root is Y-up, so undo that before attachment.
        local = local @ Matrix.Rotation(math.pi / 2, 4, 'X')
        quaternion = local.to_quaternion()
        result[side] = {'position': list(local.translation), 'quaternion': [quaternion.x, quaternion.y, quaternion.z, quaternion.w]}
    override_path = OUT / 'grip-overrides.json'
    if override_path.exists():
        for side, transform in json.loads(override_path.read_text()).items():
            if side not in result or len(transform.get('position', [])) != 3 or len(transform.get('quaternion', [])) != 4:
                raise ValueError('Invalid authored stone axe grip override')
            values = transform['position'] + transform['quaternion']
            if not all(math.isfinite(value) for value in values) or abs(math.sqrt(sum(value * value for value in transform['quaternion'])) - 1) > .001:
                raise ValueError('Invalid authored stone axe grip transform')
            result[side] = transform
    return result


def setup_review(scene):
    scene.render.engine = 'BLENDER_EEVEE'
    scene.render.resolution_x = 640
    scene.render.resolution_y = 640
    scene.render.resolution_percentage = 100
    scene.world = bpy.data.worlds.new('Equipment forest backdrop')
    scene.world.use_nodes = True
    scene.world.node_tree.nodes['Background'].inputs[0].default_value = (.055, .08, .048, 1)
    scene.world.node_tree.nodes['Background'].inputs[1].default_value = .6
    bpy.ops.object.light_add(type='AREA', location=(-3,-4,6))
    light = bpy.context.object
    light.data.energy = 500
    light.data.size = 4
    light.rotation_euler = (Vector((0,0,1)) - light.location).to_track_quat('-Z','Y').to_euler()
    bpy.ops.object.camera_add(location=(-3,-5,3.8))
    camera = bpy.context.object
    camera.rotation_euler = (Vector((0,0,1)) - camera.location).to_track_quat('-Z','Y').to_euler()
    camera.data.type = 'ORTHO'
    camera.data.ortho_scale = 2.6
    scene.camera = camera


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    scene = bpy.data.scenes.new('Stone axe travel grip')
    bpy.context.window.scene = scene
    scene.render.fps = 50
    scene.frame_start, scene.frame_end = 1, 49
    axe, source_triangles = build_axe()
    rig, mesh, low = load_character(scene)
    authored, _ = author_arms(rig, scene)
    transforms = bindings(rig)
    activate(rig)
    mesh.select_set(True)
    bpy.ops.export_scene.gltf(filepath=str(OUT / 'animation-donor.glb'), export_format='GLB',
        use_selection=True, use_active_scene=True, export_animations=True, export_animation_mode='NLA_TRACKS',
        export_frame_range=False, export_force_sampling=True, export_anim_slide_to_zero=True,
        export_extras=True, export_rest_position_armature=True, export_yup=True)
    rig.animation_data.action = None
    for track in rig.animation_data.nla_tracks:
        track.mute = track.name not in ('walk', 'axe_walk_r')
    scene.frame_set(1)
    bpy.context.view_layer.update()
    transform = transforms['r']
    quaternion = transform['quaternion']
    local = Matrix.Translation(Vector(transform['position'])) @ Quaternion((quaternion[3], *quaternion[:3])).to_matrix().to_4x4()
    axe.parent = rig
    axe.parent_type = 'BONE'
    axe.parent_bone = 'hand.r'
    # Blender bone parenting uses the tip; game sockets use the joint origin.
    axe.matrix_parent_inverse = Matrix.Translation((0, -rig.data.bones['hand.r'].length, 0))
    axe.matrix_basis = local @ Matrix.Rotation(-math.pi / 2,4,'X')
    bpy.context.view_layer.update()
    setup_review(scene)
    samples = []
    for frame in range(1, 50, 6):
        scene.frame_set(frame)
        bpy.context.view_layer.update()
        grip = (rig.matrix_world @ rig.pose.bones['hand.r'].matrix @ axe.matrix_basis).translation
        error = (axe.matrix_world.translation - grip).length
        if error > .0001:
            raise RuntimeError(f'Blender grip attachment drifts at frame {frame}: {error}')
        samples.append({'frame': frame, 'grip_error_m': error, 'position': list(axe.matrix_world.translation)})
    (OUT / 'grip-validation.json').write_text(json.dumps(samples, indent=2)+'\n')
    scene.frame_set(7)
    bpy.data.libraries.write(str(OUT / 'stone_axe_review.blend'), {scene}, fake_user=True, compress=True)
    report = {'source_triangles': source_triangles, 'triangles': sum(len(p.vertices)-2 for p in axe.data.polygons),
              'length_m': .66, 'texture_size': 512, 'bytes': RUNTIME.stat().st_size,
              'bindings': transforms, 'animations': [name for name, _ in authored]}
    (OUT / 'export-report.json').write_text(json.dumps(report, indent=2)+'\n')
    scene.render.filepath = str(OUT / 'travel-grip.png')
    bpy.ops.render.render(write_still=True)
    return report
