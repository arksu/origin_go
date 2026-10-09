"""Read-only regression checks for a saved, body-grounded Mixamo import.

Run Blender on the saved source with --python-exit-code 1 --python this file
-- --recipe path/to/fall_down.import.json. An optional --baseline blend compares
all pre-existing source actions and model data; --report writes authoring evidence.
"""
import argparse
import hashlib
import itertools
import json
import math
import sys
from pathlib import Path

import bpy

sys.path.insert(0, str(Path(__file__).resolve().parent))
from retarget_clip import body_bounds


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def matrix_values(matrix):
    return [value for row in matrix for value in row]


def source_snapshot():
    actions = {}
    for action in bpy.data.actions:
        curves = []
        for layer in action.layers:
            for strip in layer.strips:
                for bag in strip.channelbags:
                    for curve in bag.fcurves:
                        curves.append([curve.data_path, curve.array_index, curve.extrapolation,
                                       [[*key.co, *key.handle_left, *key.handle_right, key.interpolation,
                                         key.handle_left_type, key.handle_right_type] for key in curve.keyframe_points]])
        actions[action.name] = digest(sorted(curves))
    objects = []
    for obj in bpy.data.objects:
        content = [obj.name, obj.type, matrix_values(obj.matrix_world),
                   obj.parent.name if obj.parent else None, sorted(c.name for c in obj.users_collection)]
        if obj.type == 'ARMATURE':
            content.append([[bone.name, bone.parent.name if bone.parent else None,
                             matrix_values(bone.matrix_local), bone.use_deform] for bone in obj.data.bones])
        elif obj.type == 'MESH':
            content.extend([[[*vertex.co, [[group.group, group.weight] for group in vertex.groups]]
                             for vertex in obj.data.vertices],
                            [list(polygon.vertices) for polygon in obj.data.polygons],
                            [group.name for group in obj.vertex_groups],
                            [material.name if material else None for material in obj.data.materials]])
        objects.append(content)
    return {'actions': actions, 'modelContract': digest(sorted(objects))}


def pose_snapshot(target):
    return [matrix_values(bone.matrix) for bone in target.pose.bones]


def projected_envelopes(bounds):
    result = []
    for direction, name in enumerate(('NE', 'E', 'SE', 'S', 'SW', 'W', 'NW', 'N')):
        angle = (direction - 1) * math.pi / 4
        yaw = math.atan2(math.cos(angle), math.sin(angle) / 0.5)
        points = [((math.cos(yaw) * x - math.sin(yaw) * y) * 96 / 1.94,
                   (-math.cos(math.pi / 6) * z - 0.5 * (math.sin(yaw) * x + math.cos(yaw) * y)) * 96 / 1.94)
                  for x, y, z in itertools.product(*zip(*bounds))]
        result.append({'heading': name, 'minimum': [min(p[axis] for p in points) for axis in range(2)],
                       'maximum': [max(p[axis] for p in points) for axis in range(2)]})
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--recipe', type=Path, required=True)
    parser.add_argument('--baseline', type=Path)
    parser.add_argument('--report', type=Path)
    args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:])
    recipe = json.loads(args.recipe.read_text())
    source = (args.recipe.resolve().parent / recipe['source']).resolve()
    if Path(bpy.data.filepath).resolve() != source or recipe.get('grounding') != 'body':
        raise ValueError('Open the saved source of a body-grounded import recipe')
    if args.baseline:
        bpy.ops.wm.open_mainfile(filepath=str(args.baseline.resolve()))
        expected = source_snapshot()
        bpy.ops.wm.open_mainfile(filepath=str(source))
    else:
        reference = args.recipe.with_name(recipe['action'] + '.validation.json')
        reference_report = json.loads(reference.read_text())
        expected = reference_report['preservedSource']
    snapshot = source_snapshot()
    for name, fingerprint in expected['actions'].items():
        if name != recipe['action'] and snapshot['actions'].get(name) != fingerprint:
            raise ValueError(f'Pre-existing action changed: {name}')
    if snapshot['modelContract'] != expected['modelContract']:
        raise ValueError('Existing source model, meshes, weights, rig or sockets changed')
    if set(snapshot['actions']) != set(expected['actions']) | {recipe['action']}:
        raise ValueError('Import changed the source action set beyond its declared action')
    target = bpy.data.objects[recipe['rig']]
    target.animation_data.action = bpy.data.actions[recipe['action']]
    action = target.animation_data.action
    target.animation_data.action_slot = action.slots[0]
    start, end = action.frame_range
    count = recipe['range']['end'] - recipe['range']['start'] + 1
    if (start, end) != (1, count) or recipe.get('loop') or recipe.get('locomotion'):
        raise ValueError('Body clip must retain its inclusive range without looping or locomotion')
    meshes = [bpy.data.objects[name] for name in recipe['ground_meshes']]
    samples = []
    for half_frame in range(2, count * 2 + 1):
        frame = half_frame / 2
        bpy.context.scene.frame_set(int(frame), subframe=frame % 1)
        minimum, maximum = body_bounds(target, meshes)
        samples.append({'frame': frame, 'minimum': minimum, 'maximum': maximum,
                        'root': list(target.pose.bones[recipe['root_bone']].head)})
    terminal = pose_snapshot(target)
    for held_frame in (count + 1, count + 120):
        bpy.context.scene.frame_set(held_frame)
        if pose_snapshot(target) != terminal:
            raise ValueError('Terminal pose does not hold exactly after the last frame')
    minimum = [min(sample['minimum'][axis] for sample in samples) for axis in range(3)]
    maximum = [max(sample['maximum'][axis] for sample in samples) for axis in range(3)]
    clearance = recipe.get('body_clearance', 0.005)
    if minimum[2] < -0.005 or any(abs(samples[index]['minimum'][2] - clearance) > 0.005 for index in (0, -1)):
        raise ValueError('Body floor penetration or initial/terminal hovering exceeds 5 mm')
    report = {'action': recipe['action'], 'durationSeconds': (count - 1) / recipe['fps'],
              'donorSha256': hashlib.sha256((args.recipe.parent / recipe['donor']).read_bytes()).hexdigest(),
              'preservedSource': expected, 'terminalPoseSha256': digest(terminal),
              'bounds': [minimum, maximum], 'projectedEnvelopesPx': projected_envelopes([minimum, maximum]),
              'samples': samples}
    if report['donorSha256'] != recipe['donor_sha256']:
        raise ValueError('Donor provenance hash differs from the recipe')
    if not args.baseline and report['terminalPoseSha256'] != reference_report['terminalPoseSha256']:
        raise ValueError('Saved terminal body pose differs from its approved sample')
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + '\n')
    print('BODY_VALIDATION=' + json.dumps({'action': recipe['action'], 'preservedActions': len(expected['actions']),
                                         'samples': len(samples), 'bounds': report['bounds'],
                                         'firstMinimumZ': samples[0]['minimum'][2],
                                         'lastMinimumZ': samples[-1]['minimum'][2],
                                         'terminalPoseSha256': report['terminalPoseSha256']}))


if __name__ == '__main__':
    main()
