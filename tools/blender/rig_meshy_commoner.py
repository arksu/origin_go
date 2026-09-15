"""Rig the supplied Meshy character through the running Blender MCP session.

Run main() from MCP. The donor scene and supplied GLB remain unmodified.
The generated wrap is welded into the mesh, so it remains part of the base.
"""
from pathlib import Path
import json
import math
import runpy
import bpy
import bmesh
from mathutils import Vector, Matrix, Quaternion

ROOT = Path('/Users/park/projects/origin_go')
OUT = ROOT / 'art_source/characters/male_commoner_v4'
SOURCE = OUT / 'source/meshy_character.glb'
DONOR = ROOT / 'art_source/characters/male_commoner_v3/realtime/male_commoner_realtime.blend'
CLIPS = ('idle', 'walk', 'carry_idle', 'carry_walk')
WALK_STRIDE_SCALE = 1.25 * 1.15
CARRY_INWARD_TWIST = math.radians(90)


def activate(obj):
    bpy.ops.object.select_all(action='DESELECT')
    obj.hide_set(False)
    obj.select_set(True)
    bpy.context.view_layer.objects.active = obj


def read_motion():
    with bpy.data.libraries.load(str(DONOR), link=False) as (source, target):
        target.objects = ['commoner_rig']
    rig = target.objects[0]
    bpy.context.scene.collection.objects.link(rig)
    for track in rig.animation_data.nla_tracks:
        track.mute = True
    actions = {track.name: track.strips[0].action for track in rig.animation_data.nla_tracks}
    clips = {}
    for name in CLIPS:
        rig.animation_data.action = actions[name]
        samples = []
        for frame in range(1, 50):
            bpy.context.scene.frame_set(frame)
            samples.append({bone.name: bone.matrix.copy() for bone in rig.pose.bones})
        clips[name] = samples
    parents = {bone.name: bone.parent.name if bone.parent else None for bone in rig.data.bones}
    bpy.data.objects.remove(rig, do_unlink=True)
    return clips, parents


def import_mesh():
    previous = set(bpy.data.objects)
    bpy.ops.import_scene.gltf(filepath=str(SOURCE))
    meshes = [obj for obj in set(bpy.data.objects) - previous if obj.type == 'MESH']
    if len(meshes) != 1:
        raise RuntimeError('Expected one mesh in the supplied source')
    mesh = meshes[0]
    activate(mesh)
    bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
    bottom = min(vertex.co.z for vertex in mesh.data.vertices)
    factor = 1.94 / mesh.dimensions.z
    for vertex in mesh.data.vertices:
        vertex.co = Vector((vertex.co.x * factor, vertex.co.y * factor, (vertex.co.z - bottom) * factor))
    # glTF duplicates UV-seam vertices; weld positions while retaining loop UVs.
    editable = bmesh.new()
    editable.from_mesh(mesh.data)
    bmesh.ops.remove_doubles(editable, verts=list(editable.verts), dist=.00001)
    editable.to_mesh(mesh.data)
    editable.free()
    mesh.name = 'meshy_commoner'
    for material in mesh.data.materials:
        material.name = 'Meshy painted character'
        material['region'] = 'textured'
        shader = material.node_tree.nodes.get('Principled BSDF')
        shader.inputs['Metallic'].default_value = 0
        shader.inputs['Roughness'].default_value = .85
        # Keep full PBR maps in the source, omit maps unused by the pixel shader.
        for node in list(material.node_tree.nodes):
            if node.type == 'TEX_IMAGE':
                is_color = any(link.to_socket == shader.inputs['Base Color'] for link in node.outputs['Color'].links)
                if is_color:
                    node.image.scale(1024, 1024)
                    node.image.pack()
                else:
                    material.node_tree.nodes.remove(node)
    return mesh


def make_rig(reference, parents):
    joints = {
        'pelvis': ((0, .025, 1.015), (0, .02, 1.14)),
        'spine': ((0, .02, 1.14), (0, .015, 1.38)),
        'chest': ((0, .015, 1.38), (0, .005, 1.62)),
        'head': ((0, .005, 1.62), (0, .005, 1.88)),
    }
    for side, sign in (('l', 1), ('r', -1)):
        side_joints = {
            'clavicle': ((.025, .005, 1.57), (.23, .005, 1.51)),
            'upper_arm': ((.23, .005, 1.51), (.315, -.005, 1.285)),
            'forearm': ((.315, -.005, 1.285), (.375, -.012, 1.065)),
            'hand': ((.375, -.012, 1.065), (.387, -.033, .945)),
            'thigh': ((.135, .025, 1.01), (.235, .005, .565)),
            'shin': ((.235, .005, .565), (.30, .025, .12)),
            'foot': ((.30, .025, .12), (.30, -.085, .05)),
            'toes': ((.30, -.085, .05), (.30, -.16, .04)),
        }
        for name, endpoints in side_joints.items():
            joints[f'{name}.{side}'] = tuple((x * sign, y, z) for x, y, z in endpoints)
    rig = bpy.data.objects.new('commoner_rig', bpy.data.armatures.new('Commoner fitted Meshy skeleton'))
    bpy.context.scene.collection.objects.link(rig)
    activate(rig)
    bpy.ops.object.mode_set(mode='EDIT')
    for name in parents:
        bone = rig.data.edit_bones.new(name)
        if name in joints:
            bone.head, bone.tail = joints[name]
            bone.align_roll(reference[name].to_3x3().col[2])
        else:
            # Fists have no separate finger geometry. Keep animation/socket names
            # but deform these vertices with the hand instead of overlapping bones.
            hand = rig.data.edit_bones[f'hand.{name[-1]}']
            bone.head = hand.tail
            bone.tail = hand.tail + Vector((0, -.015, -.015))
            bone.use_deform = False
        if parents[name]:
            bone.parent = rig.data.edit_bones[parents[name]]
    bpy.ops.object.mode_set(mode='OBJECT')
    rig.show_in_front = True
    return rig


def smooth_range(lower, upper, value):
    blend = max(0., min(1., (value - lower) / (upper - lower)))
    return blend * blend * (3 - 2 * blend)


def correct_arm_weights(mesh, rig):
    """Constrain heat diffusion to anatomical regions, with smooth joint blends.

    The welded scan lets heat travel from the arm into chest and from wrist
    into hip. Explicit limb masks prevent either region being dragged by it.
    """
    # Below the armpits, each arm is a separate connected surface island.
    # Use topology here: a spatial falloff alone can bind an inner thumb to hip.
    neighbours = [[] for _ in mesh.data.vertices]
    for edge in mesh.data.edges:
        first, second = edge.vertices
        if mesh.data.vertices[first].co.z < 1.40 and mesh.data.vertices[second].co.z < 1.40:
            neighbours[first].append(second)
            neighbours[second].append(first)
    arm_vertices = set()
    pending = [v.index for v in mesh.data.vertices if abs(v.co.x) > .33 and 1.10 < v.co.z < 1.20]
    while pending:
        index = pending.pop()
        if index in arm_vertices:
            continue
        arm_vertices.add(index)
        pending.extend(neighbours[index])
    if not arm_vertices or any(abs(mesh.data.vertices[index].co.x) < .20 for index in arm_vertices):
        raise RuntimeError('Unable to separate lower arm islands from the torso')
    arm_indices = {group.index for group in mesh.vertex_groups if any(part in group.name for part in ('arm.', 'hand.', 'clavicle.', 'finger'))}
    for vertex in mesh.data.vertices:
        x, y, z = vertex.co
        if not .78 < z < 1.64:
            continue
        side = 'l' if x > 0 else 'r'
        lateral = abs(x)
        # Below the armpit the forearm is fully separated from the waist.
        boundary = .20 if z > 1.20 else .285
        arm_mask = smooth_range(boundary - .025, boundary + .035, lateral)
        if z < 1.40:
            island_mask = 1. if vertex.index in arm_vertices else 0.
            transition = smooth_range(1.35, 1.40, z)
            arm_mask = island_mask * (1 - transition) + arm_mask * transition
        elif lateral < .16:
            arm_mask = 0.
        current = {mesh.vertex_groups[g.group].name: g.weight for g in vertex.groups if g.group not in arm_indices}
        if not arm_mask and not any(g.group in arm_indices for g in vertex.groups):
            continue
        # Interpolate across planes perpendicular to each joint's limb axis.
        shoulder = rig.data.bones[f'upper_arm.{side}'].head_local
        elbow = rig.data.bones[f'forearm.{side}'].head_local
        wrist = rig.data.bones[f'hand.{side}'].head_local
        upper_axis = (elbow - shoulder).normalized()
        lower_axis = (wrist - elbow).normalized()
        elbow_blend = smooth_range(-.045, .045, (vertex.co - elbow).dot((upper_axis + lower_axis).normalized()))
        wrist_blend = smooth_range(-.035, .035, (vertex.co - wrist).dot(lower_axis))
        shoulder_blend = smooth_range(-.045, .065, (vertex.co - shoulder).dot(upper_axis))
        weights = {
            f'clavicle.{side}': (1 - shoulder_blend) * (1 - elbow_blend),
            f'upper_arm.{side}': shoulder_blend * (1 - elbow_blend),
            f'forearm.{side}': elbow_blend * (1 - wrist_blend),
            f'hand.{side}': elbow_blend * wrist_blend,
        }
        total = sum(current.values())
        if total < .00001:
            current = {'chest' if z > 1.30 else 'spine' if z > 1.18 else 'pelvis': 1.}
            total = 1.
        weights = {name: weight * arm_mask for name, weight in weights.items()}
        for name, weight in current.items():
            weights[name] = weight / total * (1 - arm_mask)
        for group in mesh.vertex_groups:
            group.remove([vertex.index])
        for name, weight in weights.items():
            if weight > .000001:
                mesh.vertex_groups[name].add([vertex.index], weight, 'REPLACE')


def bind(mesh, rig):
    activate(mesh)
    rig.select_set(True)
    bpy.context.view_layer.objects.active = rig
    bpy.ops.object.parent_set(type='ARMATURE_AUTO')
    for modifier in mesh.modifiers:
        if modifier.type == 'ARMATURE':
            modifier.use_deform_preserve_volume = False
    correct_arm_weights(mesh, rig)
    activate(mesh)
    bpy.ops.object.vertex_group_limit_total(limit=4)
    bpy.ops.object.vertex_group_normalize_all(lock_active=False)
    invalid = [vertex.index for vertex in mesh.data.vertices if abs(sum(group.weight for group in vertex.groups) - 1) > .002]
    if invalid:
        raise RuntimeError(f'Heat binding left {len(invalid)} invalid vertices: {invalid[:10]}')
    mesh['skinning'] = 'linear'


def fit_carry(rig, desired, rest):
    # Longer torso/shorter arms need straighter elbows to put the fists above
    # the crown. Use untwisted arm swings and preserve locomotion below the chest.
    chest_delta = desired['chest'] @ rest['chest'].inverted()
    for side, sign in (('l', 1), ('r', -1)):
        clavicle = f'clavicle.{side}'
        clavicle_head = chest_delta @ rest[clavicle].translation
        clavicle_rotation = chest_delta.to_quaternion() @ Matrix.Rotation(-sign * .48, 3, 'Y').to_quaternion() @ rest[clavicle].to_quaternion()
        desired[clavicle] = Matrix.LocRotScale(clavicle_head, clavicle_rotation, Vector((1, 1, 1)))
        shoulder = (desired[clavicle] @ rest[clavicle].inverted()) @ rest[f'upper_arm.{side}'].translation
        elbow = shoulder + Vector((sign * .055, -.012, .234)).normalized() * rig.data.bones[f'upper_arm.{side}'].length
        # Continue upward/outward instead of folding the forearms toward the
        # head. A shallow elbow bend gives the reference's relaxed overhead Y.
        wrist = elbow + Vector((sign * .005, .012, .228)).normalized() * rig.data.bones[f'forearm.{side}'].length
        tip = wrist + (wrist - elbow).normalized() * rig.data.bones[f'hand.{side}'].length
        for name, start, end in ((f'upper_arm.{side}', shoulder, elbow), (f'forearm.{side}', elbow, wrist), (f'hand.{side}', wrist, tip)):
            # Independent rest-to-overhead swings choose different twist axes
            # near 180 degrees, collapsing the skinned wrist. Transport the rest
            # orientation through the parent before applying the small joint swing.
            parent = rig.data.bones[name].parent.name
            parent_delta = desired[parent].to_quaternion() @ rest[parent].to_quaternion().inverted()
            rotation = parent_delta @ rest[name].to_quaternion()
            rotation = (rotation @ Vector((0, 1, 0))).rotation_difference((end - start).normalized()) @ rotation
            if name.startswith(('upper_arm.', 'forearm.')):
                # Share the inward turn across the arm so the wrist keeps its
                # volume instead of absorbing the entire axial rotation.
                rotation = Quaternion((end - start).normalized(), -sign * CARRY_INWARD_TWIST / 2) @ rotation
            desired[name] = Matrix.LocRotScale(start, rotation, Vector((1, 1, 1)))


def fit_walk_stance(rig, desired, stride_scale=1.0):
    """Keep the donor foot timing/height but place steps beneath the hips."""
    pelvis_x = desired['pelvis'].translation.x
    targets = {}
    lowering = 0.
    for side, sign in (('l', 1), ('r', -1)):
        target = desired[f'foot.{side}'].translation.copy()
        target.x = pelvis_x + sign * .105
        target.y = .025 + (target.y - .025) * stride_scale
        targets[side] = target
        hip = desired[f'thigh.{side}'].translation
        reach = rig.data.bones[f'thigh.{side}'].length + rig.data.bones[f'shin.{side}'].length - .004
        horizontal_squared = (hip.x - target.x) ** 2 + (hip.y - target.y) ** 2
        if horizontal_squared >= reach * reach:
            raise RuntimeError('Stride exceeds the leg reach')
        lowering = max(lowering, hip.z - target.z - math.sqrt(reach * reach - horizontal_squared))
    # Keep planted feet at their original height while extending the stride.
    # A small pelvis adjustment avoids locking or stretching the knee.
    for matrix in desired.values():
        matrix.translation.z -= lowering
    for side, sign in (('l', 1), ('r', -1)):
        thigh, shin, foot, toes = [f'{part}.{side}' for part in ('thigh', 'shin', 'foot', 'toes')]
        hip = desired[thigh].translation.copy()
        previous_ankle = desired[foot].translation.copy()
        ankle = targets[side]
        axis = ankle - hip
        distance = axis.length
        upper, lower = rig.data.bones[thigh].length, rig.data.bones[shin].length
        if distance > upper + lower - .0001:
            raise RuntimeError(f'Walk foot unreachable: {side}, {distance}')
        axis.normalize()
        along = (upper * upper - lower * lower + distance * distance) / (2 * distance)
        pole = Vector((0, -1, 0))
        pole = (pole - axis * pole.dot(axis)).normalized()
        knee = hip + axis * along + pole * math.sqrt(max(0, upper * upper - along * along))
        for name, start, end in ((thigh, hip, knee), (shin, knee, ankle)):
            rotation = desired[name].to_quaternion()
            rotation = (rotation @ Vector((0, 1, 0))).rotation_difference((end - start).normalized()) @ rotation
            desired[name] = Matrix.LocRotScale(start, rotation, Vector((1, 1, 1)))
        for name in (foot, toes):
            desired[name].translation += ankle - previous_ankle


def retarget(rig, clips):
    reference = clips['idle'][0]
    rest = {bone.name: bone.matrix_local.copy() for bone in rig.data.bones}
    actions = []
    for name, samples in clips.items():
        action = bpy.data.actions.new('Meshy ' + name)
        rig.animation_data_create()
        rig.animation_data.action = action
        previous = {}
        for index, sample in enumerate(samples):
            bpy.context.scene.frame_set(index + 1)
            desired = {}
            for bone in rig.data.bones:
                key = bone.name
                rotation = sample[key].to_quaternion() @ reference[key].to_quaternion().inverted() @ rest[key].to_quaternion()
                if bone.parent:
                    parent = bone.parent.name
                    position = (desired[parent] @ rest[parent].inverted()) @ rest[key].translation
                else:
                    position = rest[key].translation + sample[key].translation - reference[key].translation
                desired[key] = Matrix.LocRotScale(position, rotation, Vector((1, 1, 1)))
            fit_walk_stance(rig, desired, WALK_STRIDE_SCALE if name.endswith('walk') else 1.0)
            if name.startswith('carry'):
                fit_carry(rig, desired, rest)
            for bone in rig.data.bones:
                key = bone.name
                basis = rest[key].inverted()
                if bone.parent:
                    basis = basis @ rest[bone.parent.name] @ desired[bone.parent.name].inverted()
                pose = rig.pose.bones[key]
                pose.rotation_mode = 'QUATERNION'
                pose.matrix_basis = basis @ desired[key]
                if key in previous and pose.rotation_quaternion.dot(previous[key]) < 0:
                    pose.rotation_quaternion.negate()
                previous[key] = pose.rotation_quaternion.copy()
                for channel in ('location', 'rotation_quaternion', 'scale'):
                    pose.keyframe_insert(channel, frame=index + 1, group=key)
            bpy.context.view_layer.update()
        actions.append(action)
    rig.animation_data.action = None
    for name, action in zip(CLIPS, actions):
        track = rig.animation_data.nla_tracks.new()
        track.name = name
        track.strips.new(name, 1, action)
        track.mute = True
    return actions


def reduce(mesh, budget):
    activate(mesh)
    mesh.data.calc_loop_triangles()
    modifier = mesh.modifiers.new('Mobile triangle budget', 'DECIMATE')
    modifier.ratio = min(1, budget / len(mesh.data.loop_triangles))
    modifier.use_collapse_triangulate = True
    bpy.ops.object.modifier_move_to_index(modifier=modifier.name, index=0)
    bpy.ops.object.modifier_apply(modifier=modifier.name)
    for polygon in mesh.data.polygons:
        polygon.use_smooth = True
    bpy.ops.object.vertex_group_limit_total(limit=4)
    bpy.ops.object.vertex_group_normalize_all(lock_active=False)
    mesh.data.calc_loop_triangles()


def setup_review():
    scene = bpy.context.scene
    scene.render.engine = 'BLENDER_EEVEE'
    scene.render.resolution_x = 512
    scene.render.resolution_y = 640
    scene.render.resolution_percentage = 100
    scene.world = bpy.data.worlds.new('Review forest backdrop')
    scene.world.use_nodes = True
    scene.world.node_tree.nodes['Background'].inputs[0].default_value = (.055, .08, .048, 1)
    scene.world.node_tree.nodes['Background'].inputs[1].default_value = .6
    bpy.ops.object.light_add(type='AREA', location=(-3, -4, 6))
    bpy.context.object.data.energy = 450
    bpy.context.object.data.size = 4
    bpy.ops.object.camera_add(location=(0, -5, 2.5))
    camera = bpy.context.object
    camera.rotation_euler = (Vector((0, 0, 1.1)) - camera.location).to_track_quat('-Z', 'Y').to_euler()
    camera.data.type = 'ORTHO'
    camera.data.ortho_scale = 2.6
    scene.camera = camera


def render_review(rig):
    scene = bpy.context.scene
    for clip, frame, angle in [('idle', 1, 0), ('walk', 13, 0), ('walk', 7, math.pi / 2), ('carry_idle', 1, 0), ('carry_walk', 13, math.pi / 2)]:
        rig.animation_data.action = bpy.data.actions['Meshy ' + clip]
        scene.frame_set(frame)
        rig.rotation_euler.z = angle
        scene.render.filepath = str(OUT / 'review' / f'{clip}-{frame}-{round(angle * 180 / math.pi)}.png')
        bpy.ops.render.render(write_still=True)
    rig.rotation_euler.z = 0
    rig.animation_data.action = bpy.data.actions['Meshy idle']
    scene.frame_set(1)


def export(rig, mesh, low):
    rig.animation_data.action = None
    activate(rig)
    mesh.select_set(True)
    low.hide_set(False)
    low.select_set(True)
    bpy.ops.export_scene.gltf(filepath=str(OUT / 'commoner_meshy.glb'), export_format='GLB', use_selection=True,
        export_animations=True, export_animation_mode='NLA_TRACKS', export_frame_range=False,
        export_force_sampling=True, export_anim_slide_to_zero=True, export_extras=True,
        export_rest_position_armature=True, export_yup=True)
    low.hide_render = True
    low.hide_set(True)


def main():
    (OUT / 'review').mkdir(parents=True, exist_ok=True)
    # Keep the user's open scene rather than replacing it with factory settings.
    scene = bpy.data.scenes.new('Meshy commoner rig review')
    bpy.context.window.scene = scene
    clips, parents = read_motion()
    mesh = import_mesh()
    runpy.run_path(str(ROOT / 'tools/blender/brighten_meshy_skin.py'))['brighten_skin'](mesh, OUT / 'review')
    rig = make_rig(clips['idle'][0], parents)
    bind(mesh, rig)
    retarget(rig, clips)
    reduce(mesh, 16000)
    mesh['lod'] = 0
    low = mesh.copy()
    low.data = mesh.data.copy()
    low.name = 'meshy_commoner_low'
    scene.collection.objects.link(low)
    reduce(low, 5500)
    low['lod'] = 1
    scene.render.fps = 50
    scene.frame_start, scene.frame_end = 1, 49
    export(rig, mesh, low)
    setup_review()
    rig.animation_data.action = bpy.data.actions['Meshy idle']
    scene.frame_set(1)
    bpy.ops.wm.save_as_mainfile(filepath=str(OUT / 'commoner_rigged.blend'), compress=True)
    report = {'source': str(SOURCE.relative_to(ROOT)), 'donor': str(DONOR.relative_to(ROOT)), 'bones': len(rig.data.bones), 'clips': list(CLIPS), 'triangles': len(mesh.data.loop_triangles), 'low_triangles': len(low.data.loop_triangles), 'texture_size': 1024, 'garment': 'integrated in source mesh', 'skinning': 'linear'}
    (OUT / 'export-report.json').write_text(json.dumps(report, indent=2) + '\n')
    return report
