"""Retarget a Kimodo NPZ frame range using an adjacent JSON import recipe.

Blender --background source.blend --python-exit-code 1
--python tools/blender/import_kimodo_clip.py -- --recipe /path/to/import.json.
Joint indices, neutral directions and coordinate conversion are authoring data.
"""
import argparse
import json
import math
import sys
from pathlib import Path

import bpy
import numpy as np
from mathutils import Matrix, Vector

sys.path.insert(0, str(Path(__file__).resolve().parent))
from retarget_clip import bake_stationary_clip


def load_samples(recipe, directory, target):
    donor = (directory / recipe['donor']).resolve()
    with np.load(donor, allow_pickle=False) as motion:
        rotations = motion['global_rot_mats']
        positions = motion['posed_joints']
    count = recipe['joint_count']
    if rotations.ndim != 4 or rotations.shape[1:] != (count, 3, 3):
        raise ValueError(f'Expected global_rot_mats [frames, {count}, 3, 3]')
    if positions.shape != rotations.shape[:2] + (3,):
        raise ValueError('posed_joints must match rotation frames and joints')
    if not np.isfinite(rotations).all() or not np.isfinite(positions).all():
        raise ValueError('NPZ contains non-finite motion values')
    if not np.allclose(rotations @ rotations.swapaxes(-1, -2), np.eye(3), atol=0.01) or not np.allclose(
            np.linalg.det(rotations), 1, atol=0.01):
        raise ValueError('global_rot_mats must contain proper rotation matrices')
    start, end = recipe['range']['start'], recipe['range']['end']
    if type(start) is not int or type(end) is not int or not 0 <= start < end < len(rotations):
        raise ValueError('Frame range must contain at least two valid inclusive zero-based frames')
    fps = recipe['fps']
    if not isinstance(fps, (int, float)) or not math.isfinite(fps) or fps <= 0:
        raise ValueError('FPS must be finite and positive')
    conversion = np.asarray(recipe['source_to_target'], dtype=float)
    if conversion.shape != (3, 3) or not np.allclose(conversion @ conversion.T, np.eye(3)) or not np.isclose(
            np.linalg.det(conversion), 1):
        raise ValueError('source_to_target must be a proper orthonormal coordinate conversion')
    if set(recipe['bones']) != set(target.data.bones.keys()):
        raise ValueError('Bone map must cover the complete target rig')
    coordinate_rotation = Matrix(conversion.tolist()).to_quaternion()
    offsets = {}
    for name, binding in recipe['bones'].items():
        joint, tip = binding['joint'], binding['tip_joint']
        if any(type(index) is not int or not 0 <= index < count for index in [joint, tip]):
            raise ValueError(f'Invalid donor joint index for {name}')
        direction = np.asarray(binding['rest_direction'], dtype=float)
        if direction.shape != (3,) or not np.isfinite(direction).all() or np.linalg.norm(direction) < 1e-6:
            raise ValueError(f'Invalid neutral bone direction for {name}')
        # A wrong joint order or bind pose produces plausible rotations but incorrect limbs.
        observed = np.einsum('tji,tj->ti', rotations[:, joint], positions[:, tip] - positions[:, joint])
        if not np.allclose(observed, direction, atol=0.002):
            raise ValueError(f'Donor joint layout/rest direction mismatch for {name}')
        rest = target.data.bones[name].matrix_local.to_quaternion()
        alignment = (rest @ Vector((0, 1, 0))).rotation_difference(Vector(conversion @ direction))
        offsets[name] = alignment @ rest
    return [{name: coordinate_rotation @ Matrix(rotations[frame, binding['joint']].tolist()).to_quaternion() @
                   coordinate_rotation.inverted() @ offsets[name]
             for name, binding in recipe['bones'].items()} for frame in range(start, end + 1)]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--recipe', required=True, type=Path)
    args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:])
    directory = args.recipe.resolve().parent
    recipe = json.loads(args.recipe.read_text())
    source = (directory / recipe['source']).resolve()
    if Path(bpy.data.filepath).resolve() != source:
        raise ValueError('Open the source blend declared in the import recipe')
    target = bpy.data.objects[recipe['rig']]
    target.animation_data_create()
    original_action, original_slot = target.animation_data.action, target.animation_data.action_slot
    if original_action and original_action.name == recipe['action']:
        raise ValueError('Select another action before replacing the active action')
    samples = load_samples(recipe, directory, target)
    original_frame = bpy.context.scene.frame_current
    try:
        target.animation_data.action = None
        action = bake_stationary_clip(target, recipe['action'], samples,
                                      recipe['root_bone'], recipe['ground_bones'])
    finally:
        target.animation_data.action = original_action
        if original_slot:
            target.animation_data.action_slot = original_slot
        bpy.context.scene.frame_set(original_frame)
    bpy.ops.wm.save_as_mainfile(filepath=str(source))
    print(f'Baked {action.name}: NPZ {recipe["range"]["start"]}–{recipe["range"]["end"]} '
          f'-> 1–{len(samples)}, {recipe["fps"]:g} FPS')


if __name__ == '__main__':
    main()
