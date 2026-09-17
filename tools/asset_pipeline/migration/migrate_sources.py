"""One-time authored-source migration; deliberately never imported by normal builds."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import shutil
import struct
import sys

import bpy
from mathutils import Matrix, Quaternion, Vector

BASIS = Matrix(((1, 0, 0, 0), (0, 0, 1, 0), (0, -1, 0, 0), (0, 0, 0, 1)))


def require_locked_blender():
    # Inspect the executing interpreter, not an executable path or environment
    # override. Run before any source read/write in migration, audit and verification.
    lock = json.loads((Path(__file__).resolve().parent.parent / 'toolchain.lock.json').read_text())['blender']
    actual_version = '.'.join(str(component) for component in bpy.app.version)
    actual_hash = bpy.app.build_hash.decode('ascii')
    if actual_version != lock['version']:
        raise ValueError(f"Blender {lock['version']} is required; found {actual_version}")
    if actual_hash != lock['buildHash']:
        raise ValueError(f"Blender build {lock['buildHash']} is required; found {actual_hash}")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read_glb(path):
    payload = path.read_bytes()
    length = struct.unpack_from('<I', payload, 12)[0]
    return json.loads(payload[20:20 + length]), payload[28 + length:]


def accessor(document, binary, index):
    spec = document['accessors'][index]
    view = document['bufferViews'][spec['bufferView']]
    count = {'SCALAR': 1, 'VEC2': 2, 'VEC3': 3, 'VEC4': 4, 'MAT4': 16}[spec['type']]
    if spec['componentType'] != 5126:
        raise ValueError('Animation input must be float32')
    offset = view.get('byteOffset', 0) + spec.get('byteOffset', 0)
    return [struct.unpack_from('<' + 'f' * count, binary, offset + i * view.get('byteStride', count * 4))
            for i in range(spec['count'])]


def sample(times, values, interpolation, frame, quaternion=False):
    time = (frame - 1) / 50
    index = 0
    while index + 1 < len(times) and times[index + 1][0] <= time + 1e-7:
        index += 1
    first = values[index]
    if index + 1 == len(times) or interpolation == 'STEP':
        return first
    factor = max(0, min(1, (time - times[index][0]) / (times[index + 1][0] - times[index][0])))
    second = values[index + 1]
    if quaternion:
        result = Quaternion((first[3], *first[:3])).slerp(Quaternion((second[3], *second[:3])), factor)
        return (result.x, result.y, result.z, result.w)
    return tuple(a + factor * (b - a) for a, b in zip(first, second))


def collection(name, scene):
    result = bpy.data.collections.new(name)
    scene.collection.children.link(result)
    return result


def empty(name, owner, matrix=None):
    result = bpy.data.objects.new(name, None)
    owner.objects.link(result)
    result.empty_display_type = 'ARROWS'
    result.empty_display_size = .10
    if matrix is not None:
        result.matrix_world = matrix
    return result


def pack_images():
    for image in bpy.data.images:
        if image.source == 'FILE' and image.users:
            if not image.packed_file:
                image.pack()
            image.filepath = '//' + image.name + '.png'


def prepare_ui(scene, selected, center=(0, 0, .9), distance=3.6):
    bpy.context.window.scene = scene
    for obj in scene.objects:
        obj.select_set(False)
    selected.select_set(True)
    bpy.context.view_layer.objects.active = selected
    for screen in bpy.data.screens:
        for area in screen.areas:
            if area.type == 'VIEW_3D':
                space = area.spaces.active
                space.shading.type = 'MATERIAL'
                space.shading.studiolight_rotate_z = -.6
                space.region_3d.view_location = center
                space.region_3d.view_distance = distance
                space.region_3d.view_rotation = Quaternion((.82534, .56464, 0, 0))
                space.region_3d.view_perspective = 'ORTHO'
            elif area.type == 'DOPESHEET_EDITOR':
                area.spaces.active.mode = 'ACTION'
    scene.frame_start, scene.frame_end = 1, 49
    scene.render.fps = 50
    scene.frame_set(1)
    scene.world = bpy.data.worlds.new('PreviewWorld')
    scene.world.color = (.035, .075, .045)


def import_actions(rig, document, binary):
    for old in list(bpy.data.actions):
        bpy.data.actions.remove(old)
    rig.animation_data_clear()
    for clip in document['animations']:
        action = bpy.data.actions.new(clip['name'])
        action.use_fake_user = True
        action['provenance'] = 'Approved shipped runtime channels; rest-relative import, no gait regeneration'
        rig.animation_data_create()
        rig.animation_data.action = action
        channels = {}
        for channel in clip['channels']:
            name = document['nodes'][channel['target']['node']]['name']
            sampler = clip['samplers'][channel['sampler']]
            channels.setdefault(name, {})[channel['target']['path']] = (
                accessor(document, binary, sampler['input']), accessor(document, binary, sampler['output']), sampler['interpolation'])
        action['asset_pipeline_channel_mask'] = json.dumps(sorted(channels))
        for bone in rig.pose.bones:
            bone.matrix_basis = Matrix.Identity(4)
            bone.rotation_mode = 'QUATERNION'
        for name, properties in channels.items():
            bone = rig.pose.bones[name]
            rest = bone.bone.matrix_local
            if bone.parent:
                rest = bone.parent.bone.matrix_local.inverted() @ rest
            previous = None
            for frame in range(1, 50):
                translation = sample(*properties['translation'], frame)
                rotation = sample(*properties['rotation'], frame, quaternion=True)
                scale = sample(*properties['scale'], frame)
                local = Matrix.LocRotScale(Vector(translation), Quaternion((rotation[3], *rotation[:3])), Vector(scale))
                # glTF bone channels retain Blender bone-local axes. Only root bones
                # receive the Y-up basis on the left (exporter's armature sampler).
                basis = rest.inverted() @ (local if bone.parent else BASIS.inverted() @ local)
                location, orientation, scaling = basis.decompose()
                if previous is not None and previous.dot(orientation) < 0:
                    orientation.negate()
                previous = orientation.copy()
                bone.location, bone.rotation_quaternion, bone.scale = location, orientation, scaling
                for property_name in ['location', 'rotation_quaternion', 'scale']:
                    bone.keyframe_insert(property_name, frame=frame, group=name)
        for layer in action.layers:
            for strip in layer.strips:
                for bag in strip.channelbags:
                    for curve in bag.fcurves:
                        for key in curve.keyframe_points:
                            key.interpolation = 'LINEAR'
        track = rig.animation_data.nla_tracks.new()
        track.name = clip['name']
        track.strips.new(clip['name'], 1, action)
        track.mute = True
    rig.animation_data.action = bpy.data.actions['walk']
    rig.animation_data.action_slot = rig.animation_data.action.slots[0]


def commoner(root, baseline, destination):
    source = root / 'art_source/characters/male_commoner_v4/commoner_rigged.blend'
    bpy.ops.wm.open_mainfile(filepath=str(source), load_ui=False, use_scripts=False)
    rig = bpy.data.objects['commoner_rig']
    keep = {rig, bpy.data.objects['meshy_commoner'], bpy.data.objects['meshy_commoner_low']}
    for obj in list(bpy.data.objects):
        if obj not in keep:
            bpy.data.objects.remove(obj, do_unlink=True)
    scene = bpy.data.scenes.new('Male Commoner - Editable Runtime Actions')
    bpy.context.window.scene = scene
    for old in list(bpy.data.scenes):
        if old != scene:
            bpy.data.scenes.remove(old)
    for old in list(bpy.data.collections):
        bpy.data.collections.remove(old)
    exported = collection('EXPORT', scene)
    preview = collection('PREVIEW', scene)
    for obj in keep:
        exported.objects.link(obj)
        obj.hide_viewport = obj.hide_render = False
        obj.hide_set(False)
    for obj in keep - {rig}:
        obj['lod'] = 1 if obj.name.endswith('_low') else 0
        obj['skinning'] = 'linear'
        for material in obj.data.materials:
            material['region'] = 'textured'
    # Low detail remains editable/exportable without obscuring the high-detail preview.
    bpy.data.objects['meshy_commoner_low'].hide_set(True)
    path = baseline / 'artifacts/web_new/public/assets/game/characters/male_commoner/realtime/commoner_meshy.glb'
    document, binary = read_glb(path)
    import_actions(rig, document, binary)
    for side in ['l', 'r']:
        for prefix, bone_name in [('grip', 'hand'), ('forearm', 'forearm')]:
            socket = empty(f'{prefix}_{side}', exported)
            socket.parent = rig
            socket.parent_type = 'BONE'
            socket.parent_bone = f'{bone_name}.{side}'
            # Blender bone parenting attaches at the tail; runtime sockets attach at the head.
            socket.location.y = -rig.data.bones[socket.parent_bone].length
            socket.rotation_euler.x = -math.pi / 2
    info = empty('ASSET_INFO', preview)
    info['asset_id'] = 'character/male_commoner'
    info['instructions'] = 'Select commoner_rig. Action Editor selects eight editable clips. NLA tracks stay muted. Low LOD hidden in viewport only.'
    info['cycle_distance_tiles'] = 1.677975879375
    pack_images()
    bpy.data.orphans_purge(do_recursive=True)
    prepare_ui(scene, rig)
    destination.parent.mkdir(parents=True, exist_ok=True)
    bpy.ops.wm.save_as_mainfile(filepath=str(destination), check_existing=False)
    return [bone.name for bone in rig.data.bones]


def axe(root, baseline, destination, commoner_source):
    bpy.ops.wm.read_factory_settings(use_empty=False)
    for obj in list(bpy.data.objects):
        bpy.data.objects.remove(obj, do_unlink=True)
    for old in list(bpy.data.collections):
        bpy.data.collections.remove(old)
    scene = bpy.context.scene
    scene.name = 'Stone Axe - Left and Right Grip Preview'
    exported, preview = collection('EXPORT', scene), collection('PREVIEW', scene)
    # Use the authored runtime mesh; reimporting the GLB quantizes custom normals.
    source = root / 'art_source/equipment/stone_axe/stone_axe_user_grip.blend'
    with bpy.data.libraries.load(str(source), link=False) as (available, requested):
        requested.objects = ['stone_axe.003']
    mesh = requested.objects[0]
    mesh.parent = None
    mesh.matrix_world = Matrix.Identity(4)
    for owner in list(mesh.users_collection):
        owner.objects.unlink(mesh)
    exported.objects.link(mesh)
    mesh['lod'] = 0
    mesh.hide_set(True)
    bindings = json.loads((baseline / 'baseline.json').read_text())['client']['equipmentBindings']
    for side in ['l', 'r']:
        binding = bindings['left_hand' if side == 'l' else 'right_hand']
        q = binding['quaternion']
        transform = Matrix.LocRotScale(Vector(binding['position']), Quaternion((q[3], *q[:3])), Vector((1, 1, 1)))
        grip = empty(f'GRIP_{side.upper()}', exported, BASIS.inverted() @ transform.inverted() @ BASIS)
        grip['instructions'] = 'Editable equipment grip frame; inverse is the runtime socket attachment transform.'
    with bpy.data.libraries.load(str(commoner_source), link=True, relative=True) as (available, requested):
        requested.collections = ['EXPORT']
    linked = requested.collections[0]
    preview.children.link(linked)
    origin = empty('ASSET_ORIGIN', preview)
    for side in ['l', 'r']:
        socket = next(obj for obj in linked.all_objects if obj.name == f'grip_{side}')
        anchor = empty(f'Preview_{side.upper()}_Socket', preview)
        follow = anchor.constraints.new('COPY_TRANSFORMS')
        follow.target = socket
        inverse = empty(f'Preview_{side.upper()}_InverseGrip', preview)
        inverse.parent = anchor
        constraint = inverse.constraints.new('COPY_TRANSFORMS')
        constraint.target = origin
        constraint.target_space = 'CUSTOM'
        constraint.space_object = bpy.data.objects[f'GRIP_{side.upper()}']
        constraint.owner_space = 'LOCAL'
        copy = bpy.data.objects.new(f'Preview_{side.upper()}_Axe', mesh.data)
        preview.objects.link(copy)
        copy.parent = inverse
        label_data = bpy.data.curves.new(f'{side}_label', 'FONT')
        label_data.body = 'LEFT GRIP' if side == 'l' else 'RIGHT GRIP'
        label_data.size = .09
        label = bpy.data.objects.new(f'Label_{side.upper()}', label_data)
        preview.objects.link(label)
        label.location = (-.75 if side == 'l' else .22, -.2, 1.4)
        label.rotation_euler = (math.pi / 2, 0, 0)
    info = empty('ASSET_INFO', preview)
    info['asset_id'] = 'equipment/stone_axe'
    info['instructions'] = 'Edit GRIP_L / GRIP_R. Native custom-space constraints update both equipped axes live. Commoner linked by relative path. Ordinary walk is default.'
    pack_images()
    bpy.data.orphans_purge(do_recursive=True)
    prepare_ui(scene, bpy.data.objects['GRIP_R'])
    for obj in linked.all_objects:
        if obj.type == 'MESH' and obj.get('lod') == 1:
            obj.hide_set(True)
    bpy.context.view_layer.update()
    errors = {}
    for side in ['l', 'r']:
        grip = bpy.data.objects[f'GRIP_{side.upper()}']
        inverse = bpy.data.objects[f'Preview_{side.upper()}_InverseGrip']
        socket = next(obj for obj in linked.all_objects if obj.name == f'grip_{side}')
        original = grip.matrix_world.copy()
        for label, adjustment in [('saved', Matrix.Identity(4)), ('edited', Matrix.Translation((.06, -.04, .03)) @ Matrix.Rotation(.35, 4, 'Z'))]:
            grip.matrix_world = adjustment @ original
            bpy.context.view_layer.update()
            expected = socket.matrix_world @ grip.matrix_world.inverted()
            actual = inverse.evaluated_get(bpy.context.evaluated_depsgraph_get()).matrix_world
            error = max(abs(expected[r][c] - actual[r][c]) for r in range(4) for c in range(4))
            errors[f'{side}_{label}'] = error
            if error > 1e-5:
                raise ValueError(f'Native grip preview alignment failed: {side}/{label}: {error}')
        grip.matrix_world = original
        bpy.context.view_layer.update()
    for library in bpy.data.libraries:
        library.filepath = '//../../character/male_commoner/source.blend'
    bpy.ops.wm.save_as_mainfile(filepath=str(destination), check_existing=False)
    return errors


def main():
    require_locked_blender()
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', required=True)
    parser.add_argument('--baseline', required=True)
    parser.add_argument('--audit-only', action='store_true')
    parser.add_argument('--verify-only', action='store_true')
    args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:])
    root, baseline = Path(args.root).resolve(), Path(args.baseline).resolve()
    if args.verify_only:
        print('SOURCE_VERIFICATION=' + json.dumps(verify_sources(root)))
        return
    if args.audit_only:
        audit = audit_candidates(root, baseline)
        (baseline / 'candidate-audit.json').write_text(json.dumps(audit, indent=2) + '\n')
        print('CANDIDATE_AUDIT=' + json.dumps(audit))
        return
    character = root / 'art_source/character/male_commoner'
    equipment = root / 'art_source/equipment/stone_axe'
    for destination in [character / 'source.blend', equipment / 'source.blend']:
        if destination.exists():
            raise ValueError(f'Refusing to overwrite an editable canonical source: {destination}')
    required = [baseline / 'baseline.json', root / 'art_source/characters/male_commoner_v4/commoner_rigged.blend',
                root / 'art_source/characters/male_commoner_v4/source/meshy_character.glb',
                Path('/Users/park/Downloads/Meshy_AI_Stone_Axe_0915172926_texture.glb')]
    required.extend(equipment / name for name in ['stone_axe_user_grip.blend', 'stone_axe_user_grip_l.blend',
                    'stone_axe_user_idle_pose.blend', 'stone_axe_walk_pose_edit.blend', 'idle-pose-overrides.json',
                    'grip-overrides.json', 'idle-pose-edit-baseline.json', 'user-grip-validation.json', 'user-grip-validation-l.json'])
    required.extend(root / 'art_source/characters/male_commoner_v3' / name for name in [
        'README.md', 'pixel_style/walk/README.md', 'pixel_style/walk/manifest.json',
        'source/makehuman/LICENSE.ASSETS.md', 'source/makehuman/LICENSE.md',
        'source/makehuman/manifest.json', 'source/system_assets/manifest.json'])
    for source in required:
        if not source.is_file():
            raise ValueError(f'Migration input is missing: {source}')
    snapshot = json.loads((baseline / 'baseline.json').read_text())
    for index in [0, 3]:
        source = baseline / 'artifacts' / snapshot['assets'][index]['path']
        if not source.is_file() or digest(source) != snapshot['assets'][index]['fileSha256']:
            raise ValueError(f'Migration golden is missing or differs from Task 1 snapshot: {source}')
    audit = audit_candidates(root, baseline)
    bones = commoner(root, baseline, character / 'source.blend')
    errors = axe(root, baseline, equipment / 'source.blend', character / 'source.blend')
    for kind, directory, size, limits in [('character', character, 1024, {'0': 16000, '1': 5500}), ('equipment', equipment, 512, {'0': 1200})]:
        recipe = {'schema': 1, 'id': f'{kind}/{directory.name}', 'kind': kind, 'source': 'source.blend',
                  'runtimePath': f'/assets/game/{"characters" if kind == "character" else "equipment"}/{directory.name}',
                  'dependencies': {'export': [], 'preview': ['character/male_commoner'] if kind == 'equipment' else []},
                  'rig': {'object': 'commoner_rig', 'bones': bones, 'sockets': ['grip_l', 'grip_r', 'forearm_l', 'forearm_r']} if kind == 'character' else None,
                  'clips': {}, 'bindings': {},
                  'budgets': {'trianglesByLod': limits, 'textureDimensions': {'width': size, 'height': size},
                              'totalPublishedBytes': 2853888 if kind == 'character' else 499712, 'boneInfluences': 4, 'bones': 50},
                  'optimization': {'meshCompression': 'meshopt', 'texture': {'codec': 'uastc', 'quality': 2, 'width': size, 'height': size, 'mipmaps': True}}}
        if kind == 'character':
            for animation in snapshot['assets'][0]['animations']:
                name = animation['name']
                clip = {'action': name, 'range': {'start': 1, 'end': 49}, 'fps': 50, 'loop': True,
                        'playback': 'distance' if 'walk' in name else 'time'}
                if 'walk' in name:
                    clip['cycleDistanceTiles'] = snapshot['client']['cycleDistanceTiles']
                recipe['clips'][name] = clip
        else:
            for slot, binding in snapshot['client']['equipmentBindings'].items():
                recipe['bindings'][slot] = {'slot': slot, 'socket': binding['socket'],
                                            'grip': 'GRIP_L' if slot == 'left_hand' else 'GRIP_R', 'policy': {'kind': 'ordinary'}}
        (directory / 'asset.yaml').write_text(json.dumps(recipe, indent=2) + '\n')
    references(root, baseline, character, equipment, snapshot, errors, audit)
    print('MIGRATION=' + json.dumps({'bones': bones, 'previewErrors': errors}))


def audit_candidates(root, baseline):
    document, binary = read_glb(baseline / 'artifacts/web_new/public/assets/game/characters/male_commoner/realtime/commoner_meshy.glb')
    clips = {}
    for animation in document['animations']:
        channels = {}
        for channel in animation['channels']:
            sampler = animation['samplers'][channel['sampler']]
            channels.setdefault(document['nodes'][channel['target']['node']]['name'], {})[channel['target']['path']] = (
                accessor(document, binary, sampler['input']), accessor(document, binary, sampler['output']), sampler['interpolation'])
        clips[animation['name']] = channels
    candidates = [('art_source/characters/male_commoner_v4/commoner_rigged.blend', 'commoner_rig', None),
                  ('art_source/equipment/stone_axe/stone_axe_user_idle_pose.blend', 'commoner_rig.005', 'axe_idle_r'),
                  ('art_source/equipment/stone_axe/stone_axe_walk_pose_edit.blend', 'commoner_rig.006', 'axe_walk_r'),
                  ('art_source/equipment/stone_axe/stone_axe_user_grip.blend', 'commoner_rig.003', 'axe_walk_r'),
                  ('art_source/equipment/stone_axe/stone_axe_user_grip_l.blend', 'commoner_rig.004', 'axe_walk_l')]
    results = []
    for path, rig_name, clip_name in candidates:
        bpy.ops.wm.open_mainfile(filepath=str(root / path), load_ui=False, use_scripts=False)
        rig = bpy.data.objects[rig_name]
        scene = next(scene for scene in bpy.data.scenes if rig.name in scene.objects)
        bpy.context.window.scene = scene
        record = {'source': path, 'sha256': digest(root / path), 'actions': sorted(action.name for action in bpy.data.actions), 'comparisons': {}}
        chosen = [(clip_name, None)] if clip_name else [(name, f'Meshy {name}') for name in ['idle', 'walk', 'carry_idle', 'carry_walk']]
        if not clip_name:
            chosen += [(action.name.split('.')[0], action.name) for action in bpy.data.actions if action.name.startswith('axe_')]
        for name, action_name in chosen:
            if action_name:
                for bone in rig.pose.bones:
                    bone.matrix_basis = Matrix.Identity(4)
                action = bpy.data.actions[action_name]
                rig.animation_data.action = action
                rig.animation_data.action_slot = action.slots[0]
                for track in rig.animation_data.nla_tracks:
                    track.mute = True
            max_position, max_rotation = 0, 0
            for frame in [1, 7, 13, 19, 25, 31, 37, 43, 49]:
                scene.frame_set(frame)
                bpy.context.view_layer.update()
                for bone_name, properties in clips[name].items():
                    bone = rig.pose.bones[bone_name]
                    matrix = bone.parent.matrix.inverted() @ bone.matrix if bone.parent else BASIS @ bone.matrix
                    position, rotation, scaling = matrix.decompose()
                    expected_position = sample(*properties['translation'], frame)
                    expected_rotation = sample(*properties['rotation'], frame, quaternion=True)
                    expected_quaternion = Quaternion((expected_rotation[3], *expected_rotation[:3]))
                    max_position = max(max_position, (position - Vector(expected_position)).length)
                    cosine = min(1, abs(sum(float(a) * float(b) for a, b in zip(rotation, expected_quaternion))) /
                                 (math.sqrt(sum(float(a) ** 2 for a in rotation)) * math.sqrt(sum(float(a) ** 2 for a in expected_quaternion))))
                    max_rotation = max(max_rotation, 2 * math.acos(cosine))
            record['comparisons'][action_name or name] = {'runtimeClip': name, 'maxPositionError': max_position, 'maxQuaternionError': max_rotation}
        results.append(record)
    return results


def references(root, baseline, character, equipment, snapshot, errors, audit):
    records = []
    def retain(source, destination, purpose):
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, destination)
        records.append({'source': str(source.relative_to(root)) if source.is_relative_to(root) else str(source),
                        'reference': str(destination.relative_to(root)), 'sha256': digest(source),
                        'bytes': source.stat().st_size, 'purpose': purpose})
    retain(root / 'art_source/characters/male_commoner_v4/source/meshy_character.glb', character / 'references/meshy-original.glb', 'Original Meshy geometry/material reference; not MakeHuman CC0')
    retain(Path('/Users/park/Downloads/Meshy_AI_Stone_Axe_0915172926_texture.glb'), equipment / 'references/meshy-original.glb', 'Original user-supplied Meshy axe')
    for index, directory in [(0, character), (3, equipment)]:
        retain(baseline / 'artifacts' / snapshot['assets'][index]['path'], directory / 'references/approved-runtime.glb', 'Migration golden; not a normal-build input')
    for name in ['stone_axe_user_idle_pose.blend', 'stone_axe_walk_pose_edit.blend', 'stone_axe_user_grip.blend', 'stone_axe_user_grip_l.blend',
                 'idle-pose-overrides.json', 'grip-overrides.json', 'idle-pose-edit-baseline.json', 'user-grip-validation.json', 'user-grip-validation-l.json']:
        retain(equipment / name, equipment / 'references' / name, 'User edit provenance; walk edit is preserved without treating it as approved runtime')
    for relative in ['README.md', 'pixel_style/walk/README.md', 'pixel_style/walk/manifest.json', 'source/makehuman/LICENSE.ASSETS.md',
                     'source/makehuman/LICENSE.md', 'source/makehuman/manifest.json', 'source/system_assets/manifest.json']:
        retain(root / 'art_source/characters/male_commoner_v3' / relative, character / 'references/motion-donor' / relative, 'Separate motion donor provenance; does not license Meshy geometry')
    retained_snapshot = {'schemaVersion': 1, 'client': snapshot['client'], 'assets': [snapshot['assets'][0], snapshot['assets'][3]]}
    (character / 'references/approved-baseline.json').write_text(json.dumps(retained_snapshot, indent=2) + '\n')
    (character / 'references/candidate-audit.json').write_text(json.dumps(audit, indent=2) + '\n')
    provenance = {'schema': 1, 'inputs': records, 'previewMatrixErrors': errors,
                  'authoredRigSource': 'art_source/characters/male_commoner_v4/commoner_rigged.blend',
                  'authoredRigSourceSha256': digest(root / 'art_source/characters/male_commoner_v4/commoner_rigged.blend'),
                  'poseAuthority': 'Shipped runtime channels, including confirmed idle overrides, win over scene save timestamps.',
                  'budgetNote': 'Total published byte budgets provisional pending Task 5 measured optimization.',
                  'referencePolicy': 'References preserve provenance and comparison goldens only. Normal export reads source.blend and declared linked preview.'}
    for directory in [character, equipment]:
        (directory / 'references/provenance.json').write_text(json.dumps(provenance, indent=2) + '\n')


def verify_sources(root):
    result = {}
    for asset in ['character/male_commoner', 'equipment/stone_axe']:
        path = root / 'art_source' / asset / 'source.blend'
        bpy.ops.wm.open_mainfile(filepath=str(path), load_ui=True, use_scripts=False)
        recipe = json.loads((path.parent / 'asset.yaml').read_text())
        assert bpy.data.collections.get('EXPORT') and bpy.data.collections.get('PREVIEW')
        assert bpy.data.objects.get('ASSET_INFO')
        assert all(image.packed_file for image in bpy.data.images if image.source == 'FILE' and image.users)
        record = {'sourceSha256': digest(path), 'scenes': len(bpy.data.scenes),
                  'libraries': [library.filepath for library in bpy.data.libraries], 'clips': sorted(recipe['clips'])}
        assert any(area.type == 'DOPESHEET_EDITOR' and area.spaces.active.mode == 'ACTION'
                   for screen in bpy.data.screens for area in screen.areas)
        if recipe['rig']:
            rig = bpy.data.objects[recipe['rig']['object']]
            assert rig.animation_data.action.name == 'walk'
            assert all(track.mute for track in rig.animation_data.nla_tracks)
            assert all(bpy.data.actions[name].use_fake_user for name in recipe['clips'])
            assert len(rig.data.bones) == 50
            for name in recipe['rig']['sockets']:
                assert bpy.data.objects[name].parent_type == 'BONE'
            for name in recipe['clips']:
                mask = json.loads(bpy.data.actions[name]['asset_pipeline_channel_mask'])
                assert len(mask) == (3 if name.startswith('axe_') else 50)
        else:
            assert record['libraries'] == ['//../../character/male_commoner/source.blend']
            record['previewErrors'] = {}
            for side in ['L', 'R']:
                grip = bpy.data.objects[f'GRIP_{side}']
                anchor = bpy.data.objects[f'Preview_{side}_Socket']
                inverse = bpy.data.objects[f'Preview_{side}_InverseGrip']
                for frame in [1, 13, 25, 37, 49]:
                    bpy.context.scene.frame_set(frame)
                    for edited in [False, True]:
                        saved = grip.matrix_world.copy()
                        if edited:
                            grip.matrix_world = Matrix.Translation((.031, -.027, .041)) @ Matrix.Rotation(.31, 4, 'Y') @ saved
                        bpy.context.view_layer.update()
                        evaluated = bpy.context.evaluated_depsgraph_get()
                        expected = anchor.evaluated_get(evaluated).matrix_world @ grip.matrix_world.inverted()
                        actual = inverse.evaluated_get(evaluated).matrix_world
                        error = max(abs(expected[r][c] - actual[r][c]) for r in range(4) for c in range(4))
                        record['previewErrors'][f'{side}/{frame}/{edited}'] = error
                        assert error <= 1e-5, f'preview {side}/{frame}/{edited}: {error}'
                        grip.matrix_world = saved
                        bpy.context.view_layer.update()
        result[asset] = record
    return result


if __name__ == '__main__':
    main()
