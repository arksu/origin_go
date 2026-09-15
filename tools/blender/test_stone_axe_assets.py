"""Read-only regression checks for the shipped low-poly axe and arm clips."""
import struct
import json
import sys
import unittest
sys.dont_write_bytecode = True
from merge_axe_animations import ROOT, NAMES, read_glb


class StoneAxeAssets(unittest.TestCase):
    def test_custom_idle_and_unchanged_equipped_walk(self):
        game, binary = read_glb(ROOT / 'web_new/public/assets/game/characters/male_commoner/realtime/commoner_meshy.glb')
        donor, donor_binary = read_glb(ROOT / 'art_source/equipment/stone_axe/animation-donor.glb')
        overrides = json.loads((ROOT / 'art_source/equipment/stone_axe/idle-pose-overrides.json').read_text())

        def values(document, buffer, index):
            accessor = document['accessors'][index]
            view = document['bufferViews'][accessor['bufferView']]
            count = {'SCALAR': 1, 'VEC3': 3, 'VEC4': 4}[accessor['type']]
            self.assertEqual(accessor['componentType'], 5126)
            offset = view.get('byteOffset', 0) + accessor.get('byteOffset', 0)
            stride = view.get('byteStride', 4*count)
            return [struct.unpack_from('<'+'f'*count, buffer, offset+row*stride) for row in range(accessor['count'])]

        donor_clips = {clip['name']: clip for clip in donor['animations']}
        for clip in game['animations']:
            if not clip['name'].startswith('axe_'):
                continue
            for channel in clip['channels']:
                name = game['nodes'][channel['target']['node']]['name']
                path = channel['target']['path']
                sampler = clip['samplers'][channel['sampler']]
                actual = values(game, binary, sampler['output'])
                if clip['name'] in overrides:
                    for sample in actual:
                        for component, expected in zip(sample, overrides[clip['name']][name][path]):
                            self.assertAlmostEqual(component, expected, places=6)
                else:
                    original = donor_clips[clip['name']]
                    original_channel = next(c for c in original['channels'] if donor['nodes'][c['target']['node']]['name'] == name and c['target']['path'] == path)
                    original_sampler = original['samplers'][original_channel['sampler']]
                    self.assertEqual(actual, values(donor, donor_binary, original_sampler['output']))
                    self.assertEqual(values(game, binary, sampler['input']), values(donor, donor_binary, original_sampler['input']))

    def test_original_character_and_locomotion_are_unchanged(self):
        source, source_binary = read_glb(ROOT / 'art_source/characters/male_commoner_v4/commoner_meshy.glb')
        game, game_binary = read_glb(ROOT / 'web_new/public/assets/game/characters/male_commoner/realtime/commoner_meshy.glb')
        self.assertEqual(game_binary[:len(source_binary)], source_binary)
        for key in ('nodes', 'meshes', 'skins', 'materials', 'textures', 'images', 'scenes'):
            self.assertEqual(game[key], source[key], key)
        for key in ('animations', 'accessors', 'bufferViews'):
            self.assertEqual(game[key][:len(source[key])], source[key], key)
        added = game['animations'][len(source['animations']):]
        self.assertEqual({clip['name'] for clip in added}, NAMES)
        for clip in added:
            expected = {f'{joint}.{clip["name"][-1]}' for joint in ('upper_arm', 'forearm', 'hand')}
            actual = {game['nodes'][channel['target']['node']]['name'] for channel in clip['channels']}
            self.assertEqual(actual, expected)

    def test_axe_is_one_rigid_mesh_with_one_512_texture(self):
        path = ROOT / 'web_new/public/assets/game/equipment/stone_axe/stone_axe.glb'
        axe, binary = read_glb(path)
        self.assertLess(path.stat().st_size, 650_000)
        self.assertEqual(len(axe['meshes']), 1)
        self.assertNotIn('skins', axe)
        triangles = sum(axe['accessors'][primitive['indices']]['count'] // 3 for primitive in axe['meshes'][0]['primitives'])
        self.assertEqual(triangles, 1200)
        self.assertEqual(len(axe['images']), 1)
        view = axe['bufferViews'][axe['images'][0]['bufferView']]
        image = binary[view['byteOffset']:view['byteOffset']+view['byteLength']]
        self.assertEqual(image[:8], b'\x89PNG\r\n\x1a\n')
        self.assertEqual(struct.unpack_from('>II', image, 16), (512, 512))
        self.assertEqual(axe['materials'][0]['pbrMetallicRoughness']['metallicFactor'], 0)


if __name__ == '__main__':
    unittest.main()
