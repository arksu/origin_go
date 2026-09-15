"""Append only arm animation buffers; preserve all original mesh/gait bytes."""
from pathlib import Path
import copy
import json
import math
import struct

ROOT = Path(__file__).resolve().parents[2]
NAMES = {f'axe_{movement}_{side}' for movement in ('idle', 'walk') for side in ('l', 'r')}


def read_glb(path):
    raw = path.read_bytes()
    magic, version, length = struct.unpack_from('<III', raw)
    if magic != 0x46546C67 or version != 2 or length != len(raw):
        raise ValueError(f'Invalid GLB: {path}')
    json_size, kind = struct.unpack_from('<II', raw, 12)
    if kind != 0x4E4F534A:
        raise ValueError('Expected JSON chunk')
    document = json.loads(raw[20:20+json_size])
    binary_size, kind = struct.unpack_from('<II', raw, 20+json_size)
    if kind != 0x004E4942:
        raise ValueError('Expected binary chunk')
    return document, raw[28+json_size:28+json_size+binary_size]


def main():
    runtime = ROOT / 'web_new/public/assets/game/characters/male_commoner/realtime/commoner_meshy.glb'
    # Immutable phase-1 source; repeated runs never append duplicate buffers.
    original = ROOT / 'art_source/characters/male_commoner_v4/commoner_meshy.glb'
    donor = ROOT / 'art_source/equipment/stone_axe/animation-donor.glb'
    target, binary = read_glb(original)
    source, source_binary = read_glb(donor)
    binary = bytearray(binary)
    nodes = {node['name']: index for index, node in enumerate(target['nodes']) if 'name' in node}
    copied = {}
    override_path = donor.parent / 'idle-pose-overrides.json'
    overrides = json.loads(override_path.read_text()) if override_path.exists() else {}
    if set(overrides) - {'axe_idle_l', 'axe_idle_r'}:
        raise ValueError('Only idle arm poses may be overridden')

    def constant_accessor(index, values):
        value = copy.deepcopy(source['accessors'][index])
        components = {'VEC3': 3, 'VEC4': 4}.get(value['type'])
        if value['componentType'] != 5126 or len(values) != components or not all(math.isfinite(number) for number in values):
            raise ValueError('Invalid idle pose channel')
        binary.extend(b'\0' * (-len(binary) % 4))
        payload = struct.pack('<'+'f'*len(values), *values) * value['count']
        view = {'buffer': 0, 'byteOffset': len(binary), 'byteLength': len(payload)}
        binary.extend(payload)
        value['bufferView'] = len(target['bufferViews'])
        value['byteOffset'] = 0
        for bound in ('min', 'max'):
            if bound in value:
                value[bound] = values
        target['bufferViews'].append(view)
        target['accessors'].append(value)
        return len(target['accessors']) - 1

    def accessor(index):
        if index in copied:
            return copied[index]
        value = copy.deepcopy(source['accessors'][index])
        if 'sparse' in value:
            raise ValueError('Sparse animation accessors are not supported')
        view = copy.deepcopy(source['bufferViews'][value['bufferView']])
        if view['buffer'] != 0:
            raise ValueError('Expected embedded animation buffer')
        binary.extend(b'\0' * (-len(binary) % 4))
        offset = view.get('byteOffset', 0)
        chunk = source_binary[offset:offset+view['byteLength']]
        if len(chunk) != view['byteLength']:
            raise ValueError('Truncated animation buffer')
        view['byteOffset'] = len(binary)
        view['buffer'] = 0
        binary.extend(chunk)
        value['bufferView'] = len(target['bufferViews'])
        target['bufferViews'].append(view)
        copied[index] = len(target['accessors'])
        target['accessors'].append(value)
        return copied[index]

    found = set()
    for animation in source['animations']:
        name = animation['name']
        if name not in NAMES:
            continue
        side = name[-1]
        allowed = {f'{joint}.{side}' for joint in ('upper_arm', 'forearm', 'hand')}
        result = {'name': name, 'channels': [], 'samplers': []}
        for channel in animation['channels']:
            node_name = source['nodes'][channel['target']['node']]['name']
            if node_name not in allowed:
                continue
            channel = copy.deepcopy(channel)
            sampler = copy.deepcopy(animation['samplers'][channel['sampler']])
            sampler['input'] = accessor(sampler['input'])
            if name in overrides:
                sampler['output'] = constant_accessor(sampler['output'], overrides[name][node_name][channel['target']['path']])
            else:
                sampler['output'] = accessor(sampler['output'])
            channel['sampler'] = len(result['samplers'])
            channel['target']['node'] = nodes[node_name]
            result['samplers'].append(sampler)
            result['channels'].append(channel)
        if {target['nodes'][channel['target']['node']]['name'] for channel in result['channels']} != allowed:
            raise ValueError(f'Missing arm channels for {name}')
        target['animations'].append(result)
        found.add(name)
    if found != NAMES:
        raise ValueError(f'Missing animations: {NAMES-found}')
    target['buffers'][0]['byteLength'] = len(binary)
    encoded = json.dumps(target, separators=(',', ':')).encode()
    encoded += b' ' * (-len(encoded) % 4)
    binary.extend(b'\0' * (-len(binary) % 4))
    output = struct.pack('<III', 0x46546C67, 2, 28+len(encoded)+len(binary))
    output += struct.pack('<II', len(encoded), 0x4E4F534A) + encoded
    output += struct.pack('<II', len(binary), 0x004E4942) + binary
    runtime.write_bytes(output)
    print(json.dumps({'animations': sorted(found), 'runtime_bytes': len(output), 'base_mesh_and_gait_preserved': True}))


if __name__ == '__main__':
    main()
