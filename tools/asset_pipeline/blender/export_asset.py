"""Isolated raw exporter. Loads once, evaluates in memory, never saves source files."""
import argparse
import hashlib
import json
import struct
import sys
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))

import bpy
from mathutils import Matrix, Quaternion, Vector
from source_contract import (binding_metadata, canonical_hash, finite, matrix_values,
                             require, reset_scene, sample_clip, select_action,
                             validate_evaluated_transforms, validate_request, validate_source)


def read_glb(path):
    payload = path.read_bytes()
    magic, version, length = struct.unpack_from('<III', payload)
    require(magic == 0x46546C67 and version == 2 and length == len(payload), 'invalid exported GLB')
    json_length, chunk_type = struct.unpack_from('<II', payload, 12)
    require(chunk_type == 0x4E4F534A, 'missing GLB JSON')
    document = json.loads(payload[20:20 + json_length])
    offset = 20 + json_length
    binary = b''
    if offset < length:
        binary_length, binary_type = struct.unpack_from('<II', payload, offset)
        require(binary_type == 0x004E4942, 'invalid GLB binary chunk')
        binary = payload[offset + 8:offset + 8 + binary_length]
    return document, binary


def write_glb(path, document, binary):
    encoded = json.dumps(document, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()
    encoded += b' ' * (-len(encoded) % 4)
    binary += b'\0' * (-len(binary) % 4)
    size = 20 + len(encoded) + (8 + len(binary) if binary else 0)
    result = struct.pack('<IIIII', 0x46546C67, 2, size, len(encoded), 0x4E4F534A) + encoded
    if binary:
        result += struct.pack('<II', len(binary), 0x004E4942) + binary
    path.write_bytes(result)


def select_objects(objects):
    # Rebuild the view layer from the explicit export collection, ignoring saved visibility.
    for obj in bpy.context.view_layer.objects:
        obj.select_set(False)
    for obj in objects:
        obj.hide_set(False)
        obj.hide_viewport = False
        obj.hide_select = False
        obj.select_set(True)
    bpy.context.view_layer.objects.active = objects[0]


def export_glb(path, objects, recipe, clip_name=None):
    select_objects(objects)
    result = bpy.ops.export_scene.gltf(
        filepath=str(path), export_format='GLB', use_selection=True, use_active_scene=True,
        use_visible=False, use_renderable=False, export_yup=True,
        export_apply=True, export_extras=False, export_cameras=False,
        export_lights=False, export_skins=True, export_def_bones=False,
        export_all_influences=False, export_influence_nb=recipe['budgets']['boneInfluences'],
        export_rest_position_armature=True, export_current_frame=False,
        export_armature_object_remove=False, export_leaf_bone=False,
        # ACTIVE_ACTIONS intersects the requested range with authored keys. SCENE
        # bakes the selected action including held frames outside those keys.
        export_animations=clip_name is not None, export_animation_mode='SCENE',
        export_anim_scene_split_object=False,
        export_anim_single_armature=False, export_reset_pose_bones=True,
        export_force_sampling=True, export_bake_animation=True,
        export_frame_range=True, export_frame_step=1,
        export_anim_slide_to_zero=True, export_optimize_animation_size=False,
        export_nla_strips_merged_animation_name=clip_name or 'unused',
    )
    require(result == {'FINISHED'}, f'glTF export failed: {path.name}')
    document, binary = read_glb(path)
    validate_document(document, binary)
    return document, binary


def node_matrix(node):
    if 'matrix' in node:
        values = node['matrix']
        return Matrix([[values[column * 4 + row] for column in range(4)] for row in range(4)])
    rotation = node.get('rotation', [0, 0, 0, 1])
    return Matrix.LocRotScale(Vector(node.get('translation', [0, 0, 0])),
                             Quaternion((rotation[3], *rotation[:3])), Vector(node.get('scale', [1, 1, 1])))


def skeleton_contract(document, rig):
    if rig is None:
        return None
    nodes = document.get('nodes', [])
    by_name = {node['name']: index for index, node in enumerate(nodes) if 'name' in node}
    parents = {child: index for index, node in enumerate(nodes) for child in node.get('children', [])}
    bones = []
    for bone in sorted(rig.data.bones, key=lambda entry: entry.name):
        require(bone.name in by_name, f'export omitted rig bone {bone.name}')
        index = by_name[bone.name]
        parent = parents.get(index)
        bones.append({'name': bone.name, 'parent': nodes[parent].get('name') if parent is not None else None,
                      'restMatrix': matrix_values(node_matrix(nodes[index]))})
    require(rig.name in by_name, 'export omitted rig object')
    contract = {'object': rig.name, 'objectRestMatrix': matrix_values(node_matrix(nodes[by_name[rig.name]])), 'bones': bones}
    return {**contract, 'fingerprint': canonical_hash(contract)}


def validate_document(document, binary):
    for node in document.get('nodes', []):
        finite(matrix_values(node_matrix(node)), f'exported node {node.get("name")}')
    for accessor in document.get('accessors', []):
        require('sparse' not in accessor, 'unexpected sparse exporter accessor')
        if accessor['componentType'] != 5126:
            continue
        view = document['bufferViews'][accessor['bufferView']]
        components = {'SCALAR': 1, 'VEC2': 2, 'VEC3': 3, 'VEC4': 4, 'MAT4': 16}[accessor['type']]
        stride = view.get('byteStride', components * 4)
        offset = view.get('byteOffset', 0) + accessor.get('byteOffset', 0)
        for index in range(accessor['count']):
            finite(struct.unpack_from('<' + 'f' * components, binary, offset + index * stride), 'exported geometry/weights/animation')


def clip_document(document, binary, mask, name):
    require(len(document.get('animations', [])) == 1, f'expected exactly one animation for {name}')
    animation = document['animations'][0]
    animation['name'] = name
    animation['channels'] = [channel for channel in animation['channels']
                             if document['nodes'][channel['target']['node']].get('name') in mask]
    require(animation['channels'], f'empty animation after channel mask: {name}')
    used = sorted({channel['sampler'] for channel in animation['channels']})
    remap = {old: new for new, old in enumerate(used)}
    animation['samplers'] = [animation['samplers'][index] for index in used]
    for channel in animation['channels']:
        channel['sampler'] = remap[channel['sampler']]
    require(not document.get('meshes') and not document.get('images'), 'standalone clip must not have mesh/image dependencies')
    kept = [index for index, node in enumerate(document['nodes']) if 'mesh' not in node]
    node_map = {old: new for new, old in enumerate(kept)}
    document['nodes'] = [document['nodes'][index] for index in kept]
    for node in document['nodes']:
        if 'children' in node:
            node['children'] = [node_map[index] for index in node['children'] if index in node_map]
        node.pop('skin', None)
    for scene in document['scenes']:
        scene['nodes'] = [node_map[index] for index in scene['nodes'] if index in node_map]
    for channel in animation['channels']:
        channel['target']['node'] = node_map[channel['target']['node']]
    accessors = sorted({sampler[field] for sampler in animation['samplers'] for field in ['input', 'output']})
    accessor_map = {old: new for new, old in enumerate(accessors)}
    document['accessors'] = [document['accessors'][index] for index in accessors]
    for sampler in animation['samplers']:
        for field in ['input', 'output']:
            sampler[field] = accessor_map[sampler[field]]
    views = sorted({accessor['bufferView'] for accessor in document['accessors']})
    view_map = {old: new for new, old in enumerate(views)}
    compact_binary = bytearray()
    compact_views = []
    for index in views:
        view = document['bufferViews'][index].copy()
        offset = view.get('byteOffset', 0)
        compact_binary.extend(b'\0' * (-len(compact_binary) % 4))
        view['byteOffset'] = len(compact_binary)
        compact_binary.extend(binary[offset:offset + view['byteLength']])
        compact_views.append(view)
    for accessor in document['accessors']:
        accessor['bufferView'] = view_map[accessor['bufferView']]
    document['bufferViews'] = compact_views
    document['buffers'] = [{'byteLength': len(compact_binary)}]
    for field in ['meshes', 'skins', 'images', 'textures', 'materials', 'samplers']:
        document.pop(field, None)
    return bytes(compact_binary)


def texture_metadata(document, binary, directory):
    textures = []
    for index, image in enumerate(document.get('images', [])):
        require('bufferView' in image and image.get('mimeType') in {'image/png', 'image/jpeg'}, 'unsupported exported image encoding')
        view = document['bufferViews'][image['bufferView']]
        offset = view.get('byteOffset', 0)
        payload = binary[offset:offset + view['byteLength']]
        name = f'{index}.png' if image['mimeType'] == 'image/png' else f'{index}.jpg'
        (directory / name).write_bytes(payload)
        textures.append({'imageIndex': index, 'path': f'textures/{name}', 'sha256': hashlib.sha256(payload).hexdigest()})
    return textures


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--request', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:])
    request_path = Path(args.request)
    require(request_path.is_absolute() and request_path.is_file(), 'request must be an absolute JSON file')
    request = json.loads(request_path.read_text())
    output = Path(args.output)
    recipe, source, dependencies = validate_request(request, output)
    inputs = {path: hashlib.sha256(path.read_bytes()).hexdigest() for path in {source, *dependencies}}
    bpy.ops.wm.open_mainfile(filepath=str(source), load_ui=False, use_scripts=False)
    objects, rig = validate_source(recipe, dependencies)
    # A fresh scene prevents saved scene/world/collection visibility and timeline settings
    # from choosing model content. Only validated objects enter evaluation/export.
    scene = bpy.data.scenes.new('AssetPipelineExport')
    for obj in objects:
        scene.collection.objects.link(obj)
    bpy.context.window.scene = scene
    reset_scene(rig)
    validate_evaluated_transforms(objects)
    bindings = binding_metadata(recipe)
    raw = output / 'raw'
    (raw / 'animations').mkdir(parents=True)
    (raw / 'textures').mkdir()
    model_path = raw / 'model.glb'
    document, binary = export_glb(model_path, objects, recipe)
    meshes = {obj.name: obj for obj in objects if obj.type == 'MESH'}
    for node in document.get('nodes', []):
        if node.get('name') in meshes:
            obj = meshes[node['name']]
            node['extras'] = {'lod': obj['lod'], 'skinned': bool(obj['skinned'])}
            if 'skinning' in obj:
                node['extras']['skinning'] = obj['skinning']
    materials = {material.name: material for obj in meshes.values() for material in obj.data.materials}
    for material in document.get('materials', []):
        source_material = materials.get(material.get('name'))
        if source_material is not None and 'region' in source_material:
            material['extras'] = {'region': source_material['region']}
    write_glb(model_path, document, binary)
    contract = skeleton_contract(document, rig)
    nodes = {node.get('name'): node for node in document.get('nodes', [])}
    parents = {document['nodes'][child].get('name'): node.get('name') for node in document.get('nodes', []) for child in node.get('children', [])}
    sockets = {name: {'parent': parents.get(name), 'localMatrix': matrix_values(node_matrix(nodes[name]))}
               for name in (recipe['rig']['sockets'] if rig else [])}
    fingerprint_inputs = {'modelSha256': hashlib.sha256(model_path.read_bytes()).hexdigest(),
                          'bindings': bindings, 'rigFingerprint': contract['fingerprint'] if contract else None,
                          'settings': {key: recipe[key] for key in ['kind', 'rig', 'budgets', 'optimization']}}
    metadata = {'schema': 1, 'assetId': recipe['id'], 'rig': contract, 'sockets': sockets,
                'bindings': bindings, 'textures': texture_metadata(document, binary, raw / 'textures'), 'clips': {},
                'modelFingerprint': canonical_hash(fingerprint_inputs), 'modelFingerprintInputs': fingerprint_inputs}
    for name in sorted(request['selectedClips']):
        clip = recipe['clips'][name]
        mask = select_action(rig, clip)
        checks = sample_clip(rig, clip, mask)
        clip_path = raw / 'animations' / f'{name}.glb'
        clip_json, clip_binary = export_glb(clip_path, [rig], recipe, name)
        clip_binary = clip_document(clip_json, clip_binary, mask, name)
        validate_document(clip_json, clip_binary)
        clip_contract = skeleton_contract(clip_json, rig)
        require(clip_contract == contract, f'clip skeleton differs from model: {name}')
        write_glb(clip_path, clip_json, clip_binary)
        metadata['clips'][name] = {**clip, **checks, 'rig': clip_contract}
    for path, digest in inputs.items():
        require(hashlib.sha256(path.read_bytes()).hexdigest() == digest, f'source input changed during export: {path.name}')
    (raw / 'metadata.json').write_text(json.dumps(metadata, sort_keys=True, indent=2, allow_nan=False) + '\n')
    print('ASSET_PIPELINE_EXPORTED=' + recipe['id'])


if __name__ == '__main__':
    main()
