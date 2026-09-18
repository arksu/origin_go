"""Validation and canonical evaluation of an explicitly declared Blender source."""
import hashlib
import json
import math
import re
from pathlib import Path

import bpy
from mathutils import Matrix

NAME = re.compile(r'^[A-Za-z0-9][A-Za-z0-9_.-]*$')
BASIS = Matrix(((1, 0, 0, 0), (0, 0, 1, 0), (0, -1, 0, 0), (0, 0, 0, 1)))
SUPPORTED_CONSTRAINTS = {'COPY_LOCATION', 'COPY_ROTATION', 'COPY_SCALE', 'COPY_TRANSFORMS',
                         'LIMIT_LOCATION', 'LIMIT_ROTATION', 'LIMIT_SCALE', 'LIMIT_DISTANCE',
                         'DAMPED_TRACK', 'TRACK_TO', 'LOCKED_TRACK', 'STRETCH_TO', 'IK',
                         'TRANSFORM', 'CHILD_OF', 'MAINTAIN_VOLUME'}
SUPPORTED_MODIFIERS = {'ARMATURE', 'MIRROR', 'SUBSURF', 'TRIANGULATE', 'BEVEL',
                       'WEIGHTED_NORMAL', 'SOLIDIFY', 'EDGE_SPLIT', 'DECIMATE'}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def finite(values, label):
    require(all(math.isfinite(value) for value in values), f'{label} must be finite')


def matrix_values(matrix):
    return [float(matrix[row][column]) for column in range(4) for row in range(4)]


def canonical_hash(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()).hexdigest()


def absolute_file(value, label):
    require(isinstance(value, str) and Path(value).is_absolute(), f'{label} must be an absolute local path')
    path = Path(value)
    require('..' not in path.parts and path.is_file(), f'{label} must be an existing normalized file')
    require(path.resolve() == path, f'{label} must not traverse symlinks')
    return path


def validate_request(request, output):
    require(isinstance(request, dict), 'request must be an object')
    require(set(request) == {'recipe', 'selectedClips', 'resolvedDependencies'}, 'unknown or missing request fields')
    recipe = request['recipe']
    require(isinstance(recipe, dict) and recipe.get('schema') == 1, 'recipe schema must be 1')
    require(recipe.get('kind') in {'character', 'equipment', 'world_object'}, 'invalid recipe kind')
    require(isinstance(recipe.get('id'), str) and re.fullmatch(r'(character|equipment|world_object)/[a-z0-9][a-z0-9_-]*', recipe['id']), 'invalid asset id')
    source = absolute_file(recipe['source']['absolutePath'], 'source')
    require(source.suffix == '.blend', 'source must be .blend')
    require(output.is_absolute() and '..' not in output.parts, 'output must be an absolute normalized directory')
    # The worker only creates a fresh staging tree, never overwrites an existing tree.
    require(not output.exists(), 'output directory already exists')
    require(output.parent.is_dir() and output.parent.resolve() == output.parent, 'output parent must exist without symlinks')
    clips = request['selectedClips']
    require(isinstance(clips, list) and len(set(clips)) == len(clips), 'selectedClips must be unique')
    for name in clips:
        require(isinstance(name, str) and NAME.fullmatch(name) and name in recipe['clips'], 'invalid selected clip name')
    for name, clip in recipe['clips'].items():
        require(NAME.fullmatch(name) and NAME.fullmatch(clip['action']), 'invalid clip/action name')
        start, end, fps = clip['range']['start'], clip['range']['end'], clip['fps']
        require(all(isinstance(number, (int, float)) and math.isfinite(number) for number in [start, end, fps]), 'clip range/fps must be finite')
        frame_limit = bpy.types.Scene.bl_rna.properties['frame_end'].hard_max
        require(start == int(start) and end == int(end) and 0 <= start < end <= frame_limit and fps > 0, 'invalid clip range/fps')
        require(isinstance(clip['loop'], bool), 'clip.loop must be boolean')
    require(isinstance(recipe['budgets']['boneInfluences'], int) and 1 <= recipe['budgets']['boneInfluences'] <= 8, 'invalid bone influence budget')
    dependencies = request['resolvedDependencies']
    declared = set(recipe['dependencies']['export'] + recipe['dependencies']['preview'])
    require(isinstance(dependencies, list), 'resolvedDependencies must be an array')
    seen, paths = set(), set()
    for dependency in dependencies:
        require(set(dependency) == {'assetId', 'absolutePath'}, 'invalid resolved dependency fields')
        require(dependency['assetId'] in declared and dependency['assetId'] not in seen, 'undeclared or duplicate dependency')
        seen.add(dependency['assetId'])
        paths.add(absolute_file(dependency['absolutePath'], 'dependency'))
    require(seen == declared, 'missing resolved dependency inputs')
    return recipe, source, paths


def is_bundled_asset_library(path):
    local_resource_root = bpy.utils.resource_path('LOCAL')
    if not local_resource_root:
        return False
    bundled_assets = (Path(local_resource_root) / 'datafiles' / 'assets').resolve()
    return bundled_assets.is_dir() and path.is_relative_to(bundled_assets)


def validate_dependencies(allowed_paths):
    for library in bpy.data.libraries:
        path = Path(bpy.path.abspath(library.filepath)).resolve()
        # Blender can attach read-only built-in asset libraries (for example sculpt
        # brushes) while opening a source. They are part of the installed tool, not
        # inputs that should be declared in an asset recipe.
        require((path in allowed_paths and path.is_file()) or is_bundled_asset_library(path),
                f'undeclared library dependency: {library.filepath}')
    for image in bpy.data.images:
        if image.name in {'Render Result', 'Viewer Node'}:
            continue
        require(image.source in {'FILE', 'GENERATED'}, f'unsupported image input: {image.name}')
        require(bool(image.packed_file) or image.source == 'GENERATED', f'undeclared unpacked image: {image.name}; pack source images')
        require(image.size[0] > 0 and image.size[1] > 0, f'invalid image: {image.name}')
    require(not bpy.data.sounds and not bpy.data.movieclips and not bpy.data.cache_files, 'audio, movie and simulation caches are unsupported inputs')
    # Drivers may execute expressions or depend on scene state outside EXPORT.
    for collection in [bpy.data.objects, bpy.data.meshes, bpy.data.armatures, bpy.data.materials,
                       bpy.data.shape_keys, bpy.data.scenes, bpy.data.worlds, bpy.data.node_groups]:
        for block in collection:
            animation = getattr(block, 'animation_data', None)
            require(not animation or not animation.drivers, f'unsupported driver on {block.name}')
            nodes = getattr(block, 'node_tree', None)
            require(not nodes or not nodes.animation_data or not nodes.animation_data.drivers, f'unsupported node driver on {block.name}')
    for scene in bpy.data.scenes:
        scene.render.use_sequencer = False
        require(scene.rigidbody_world is None, 'unbaked rigid body simulation is unsupported')


def validate_names(objects, rig):
    seen = {}
    for name in [obj.name for obj in objects] + ([bone.name for bone in rig.data.bones] if rig else []):
        require(NAME.fullmatch(name), f'unsupported stable exported name: {name}')
        sanitized = re.sub(r'[\[\].:/]', '', re.sub(r'\s', '_', name))
        require(sanitized not in seen, f'exported name collision after Three.js sanitization: {name} and {seen.get(sanitized)}')
        seen[sanitized] = name


def validate_constraints(owner, objects, label):
    for constraint in owner.constraints:
        require(constraint.type in SUPPORTED_CONSTRAINTS, f'unsupported constraint {constraint.type} on {label}')
        for target_field, bone_field in [('target', 'subtarget'), ('pole_target', 'pole_subtarget')]:
            target = getattr(constraint, target_field, None)
            require(target is None or target in objects, f'undeclared constraint {target_field} on {label}')
            if target is not None:
                subtarget = getattr(constraint, bone_field, '')
                require(not subtarget or (target.type == 'ARMATURE' and subtarget in target.data.bones), f'invalid constraint bone on {label}')
        finite([constraint.influence], f'constraint influence on {label}')


def validate_material(material):
    require(material is not None and material.use_nodes, 'materials must use a Principled node graph')
    if 'region' in material:
        region = material['region']
        require(isinstance(region, str) and region in {'skin', 'hair', 'linen', 'eyes', 'textured', 'foliage'},
                f'unsupported material region on {material.name}')
    supported = {'OUTPUT_MATERIAL', 'BSDF_PRINCIPLED', 'TEX_IMAGE', 'NORMAL_MAP', 'SEPARATE_COLOR',
                 'MATH', 'MIX', 'RGB', 'VALUE', 'UVMAP', 'TEX_COORD', 'MAPPING', 'REROUTE'}
    for node in material.node_tree.nodes:
        require(node.type in supported, f'unsupported material node {node.type} in {material.name}')
        if node.type == 'TEX_IMAGE':
            require(node.image is not None and bool(node.image.packed_file), f'material image must be packed: {material.name}')
    require(any(node.type == 'BSDF_PRINCIPLED' for node in material.node_tree.nodes), f'missing Principled shader: {material.name}')


def validate_mesh(obj, rig, max_influences):
    require(obj.data.shape_keys is None, f'shape keys require an explicit contract: {obj.name}')
    for vertex in obj.data.vertices:
        finite(vertex.co, f'vertex on {obj.name}')
    for modifier in obj.modifiers:
        require(modifier.type in SUPPORTED_MODIFIERS, f'unsupported unbaked modifier {modifier.type} on {obj.name}')
        require(modifier.show_viewport and modifier.show_render, f'modifier visibility must agree on {obj.name}')
        if modifier.type == 'ARMATURE':
            require(rig is not None and modifier.object == rig, f'undeclared armature modifier on {obj.name}')
        dependency = getattr(modifier, 'mirror_object', None)
        require(dependency is None, f'mirror object requires baked geometry: {obj.name}')
    armatures = [modifier for modifier in obj.modifiers if modifier.type == 'ARMATURE']
    require(len(armatures) <= 1, f'multiple armature modifiers on {obj.name}')
    if armatures:
        names = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            weights = [group.weight for group in vertex.groups if names.get(group.group) in rig.data.bones and group.weight > 0]
            finite([group.weight for group in vertex.groups], f'weights on {obj.name}')
            require(weights and len(weights) <= max_influences and abs(sum(weights) - 1) < 1e-4, f'invalid skin weights on {obj.name} vertex {vertex.index}')
    for material in obj.data.materials:
        validate_material(material)
    lod = obj.get('lod', 0)
    require(isinstance(lod, int) and lod >= 0, f'invalid lod extra on {obj.name}')
    obj['lod'] = lod
    obj['skinned'] = bool(armatures)
    if 'skinning' in obj:
        skinning = obj['skinning']
        require(isinstance(skinning, str) and skinning in {'linear', 'dual_quaternion'},
                f'unsupported mesh skinning on {obj.name}')


def validate_source(recipe, dependencies):
    validate_dependencies(dependencies)
    collection = bpy.data.collections.get('EXPORT')
    require(collection is not None, 'missing EXPORT collection')
    objects = sorted(collection.all_objects, key=lambda obj: obj.name)
    require(objects and any(obj.type == 'MESH' for obj in objects), 'EXPORT needs mesh geometry')
    require(all(obj.type in {'MESH', 'ARMATURE', 'EMPTY'} for obj in objects), 'EXPORT contains unsupported object types')
    rig_spec = recipe['rig']
    rig = bpy.data.objects.get(rig_spec['object']) if rig_spec else None
    require(rig_spec is None or (rig is not None and rig.type == 'ARMATURE' and rig in objects), 'missing declared rig object')
    armatures = [obj for obj in objects if obj.type == 'ARMATURE']
    require(armatures == ([rig] if rig else []), 'EXPORT rig does not match recipe')
    require(recipe['kind'] != 'character' or rig is not None, 'character requires rig')
    validate_names(objects, rig)
    for obj in objects:
        require(obj.parent is None or obj.parent in objects, f'undeclared parent on {obj.name}')
        validate_constraints(obj, objects, obj.name)
        if obj != rig:
            require(not obj.animation_data or (obj.animation_data.action is None and not obj.animation_data.nla_tracks), f'object animation requires rig action contract: {obj.name}')
        if obj.type == 'MESH':
            validate_mesh(obj, rig, recipe['budgets']['boneInfluences'])
    if rig:
        require(set(rig_spec['bones']).issubset(rig.data.bones.keys()), 'missing required rig bone')
        for bone in rig.data.bones:
            finite(matrix_values(bone.matrix_local), f'rest bone {bone.name}')
        for bone in rig.pose.bones:
            validate_constraints(bone, objects, bone.name)
        for name in rig_spec['sockets']:
            socket = bpy.data.objects.get(name)
            require(socket in objects and socket.type == 'EMPTY', f'missing socket {name}')
            require(socket.parent == rig and socket.parent_type == 'BONE' and socket.parent_bone in rig.data.bones, f'socket {name} must be bone-parented to rig')
    for name, binding in recipe['bindings'].items():
        require(NAME.fullmatch(name), 'invalid binding name')
        grip = bpy.data.objects.get(binding['grip'])
        require(grip in objects and grip.type == 'EMPTY', f'missing grip {binding["grip"]}')
    return objects, rig


def validate_evaluated_transforms(objects):
    # Saved pose channels can make bone-parented sockets singular even when the
    # authored rest rig is valid. Inspect only the reset export scene's evaluation.
    depsgraph = bpy.context.evaluated_depsgraph_get()
    for obj in objects:
        matrix = obj.evaluated_get(depsgraph).matrix_world
        finite(matrix_values(matrix), f'transform on {obj.name}')
        require(abs(matrix.determinant()) > 1e-8, f'singular transform on {obj.name}')


def reset_scene(rig):
    for obj in bpy.data.objects:
        if obj.animation_data:
            obj.animation_data.action = None
            for track in obj.animation_data.nla_tracks:
                track.mute = True
        if obj.type == 'ARMATURE':
            for bone in obj.pose.bones:
                bone.matrix_basis = Matrix.Identity(4)
            obj.data.pose_position = 'REST'
    bpy.context.scene.frame_set(0)
    bpy.context.view_layer.update()


def action_curves(action):
    for layer in action.layers:
        for strip in layer.strips:
            for bag in strip.channelbags:
                yield from bag.fcurves


def select_action(rig, clip):
    require(rig is not None, 'clips require a rig')
    action = bpy.data.actions.get(clip['action'])
    require(action is not None, f'missing action {clip["action"]}')
    require(len(action.slots) == 1 and action.slots[0].target_id_type == 'OBJECT', 'actions require exactly one object slot')
    authored = set()
    for curve in action_curves(action):
        match = re.fullmatch(r'pose\.bones\["([^"\\]+)"\]\.(location|rotation_euler|rotation_quaternion|scale)', curve.data_path)
        require(match and match[1] in rig.data.bones, f'unsupported action channel: {curve.data_path}')
        authored.add(match[1])
        require(not curve.modifiers, 'action F-curve modifiers require explicit baking')
        for key in curve.keyframe_points:
            finite([*key.co, *key.handle_left, *key.handle_right], 'action key/handle')
    require(authored, f'empty action {action.name}')
    mask = action.get('asset_pipeline_channel_mask', list(rig.data.bones.keys()))
    if isinstance(mask, str):
        mask = json.loads(mask)
    require(isinstance(mask, (list, tuple)) or hasattr(mask, 'to_list'), 'action channel mask must be a list of bone names')
    mask = list(mask)
    require(mask and all(isinstance(name, str) for name in mask) and len(set(mask)) == len(mask), 'invalid action channel mask')
    require(set(mask).issubset(rig.data.bones.keys()) and authored.issubset(mask), 'action channel mask omits authored bone or names unknown bone')
    reset_scene(rig)
    rig.data.pose_position = 'POSE'
    rig.animation_data_create()
    rig.animation_data.action = action
    rig.animation_data.action_slot = action.slots[0]
    scene = bpy.context.scene
    scene.frame_start, scene.frame_end = int(clip['range']['start']), int(clip['range']['end'])
    scene.render.fps = max(1, round(clip['fps']))
    scene.render.fps_base = scene.render.fps / clip['fps']
    require(math.isclose(scene.render.fps / scene.render.fps_base, clip['fps'], rel_tol=1e-6), 'clip fps is outside Blender representation')
    return mask


def sample_clip(rig, clip, mask):
    start, end = clip['range']['start'], clip['range']['end']
    boundary = []
    count = 0
    for half_frame in range(int(start * 2), int(end * 2) + 1):
        frame = half_frame / 2
        bpy.context.scene.frame_set(int(frame), subframe=frame % 1)
        evaluated = rig.evaluated_get(bpy.context.evaluated_depsgraph_get())
        matrices = {}
        for bone in evaluated.pose.bones:
            matrix = bone.parent.matrix.inverted() @ bone.matrix if bone.parent else bone.matrix
            finite(matrix_values(matrix), f'sampled pose {bone.name} at {frame}')
            require(abs(matrix.determinant()) > 1e-8, f'singular sampled pose {bone.name} at {frame}')
            if bone.name in mask:
                matrices[bone.name] = matrix_values(matrix)
        if frame in {start, end}:
            boundary.append(matrices)
        count += 1
    error = max(abs(first - last) for name in mask for first, last in zip(boundary[0][name], boundary[1][name]))
    require(not clip['loop'] or error <= 1e-4, f'loop closure failed for {clip["action"]}: {error}')
    return {'sampleCount': count, 'loopClosureMaxError': error, 'durationSeconds': (end - start) / clip['fps'], 'channelMask': sorted(mask)}


def binding_metadata(recipe):
    bindings = {}
    for name, binding in sorted(recipe['bindings'].items()):
        grip = bpy.data.objects[binding['grip']].evaluated_get(bpy.context.evaluated_depsgraph_get())
        matrix = BASIS @ grip.matrix_world @ BASIS.inverted()
        location, rotation, scale = matrix.decompose()
        reconstructed = Matrix.LocRotScale(location, rotation, scale)
        require(all(abs(value - 1) < 1e-5 for value in scale), f'grip {grip.name} must have unit scale')
        require(max(abs(a - b) for a, b in zip(matrix_values(matrix), matrix_values(reconstructed))) < 1e-5, f'grip {grip.name} must not contain shear')
        bindings[name] = {**binding, 'gripMatrix': matrix_values(matrix), 'gripInverse': matrix_values(matrix.inverted())}
    return bindings
