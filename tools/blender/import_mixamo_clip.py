"""Retarget an inclusive FBX frame range using an adjacent JSON import recipe.

Blender --background source.blend --python tools/blender/import_mixamo_clip.py
-- --recipe /path/to/import.json. Only the declared action is replaced.
"""
import argparse
import json
import sys
from pathlib import Path

import bpy
from mathutils import Vector

sys.path.insert(0, str(Path(__file__).resolve().parent))
from retarget_clip import bake_stationary_clip


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--recipe', required=True, type=Path)
    args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:])
    recipe = json.loads(args.recipe.read_text())
    directory = args.recipe.resolve().parent
    source = (directory / recipe['source']).resolve()
    donor_path = (directory / recipe['donor']).resolve()
    if Path(bpy.data.filepath).resolve() != source or not donor_path.is_file():
        raise ValueError('Open the recipe source blend and provide its donor FBX')
    start, end = recipe['range']['start'], recipe['range']['end']
    if type(start) is not int or type(end) is not int or end <= start:
        raise ValueError('Frame range must contain at least two inclusive integer frames')
    target = bpy.data.objects[recipe['rig']]
    bone_map = recipe['bones']
    if set(bone_map) != set(target.data.bones.keys()):
        raise ValueError('Bone map must cover the complete target rig')
    for name in [recipe['root_bone'], *recipe['ground_bones']]:
        if name not in bone_map:
            raise ValueError(f'Unknown grounding bone: {name}')
    scene = bpy.context.scene
    original_frame = scene.frame_current
    original_timing = (scene.render.fps, scene.render.fps_base, scene.frame_start, scene.frame_end)
    original_selection = list(bpy.context.selected_objects)
    original_active = bpy.context.view_layer.objects.active
    target.animation_data_create()
    original_action, original_slot = target.animation_data.action, target.animation_data.action_slot
    if original_action and original_action.name == recipe['action']:
        raise ValueError('Select another action before replacing the active action')
    before_objects, before_actions = set(bpy.data.objects), set(bpy.data.actions)
    before_blocks = [(collection, set(collection)) for collection in
                     (bpy.data.meshes, bpy.data.armatures, bpy.data.materials, bpy.data.images)]
    bpy.ops.import_scene.fbx(filepath=str(donor_path))
    imported = set(bpy.data.objects) - before_objects
    rigs = [obj for obj in imported if obj.type == 'ARMATURE']
    if len(rigs) != 1 or not rigs[0].animation_data or not rigs[0].animation_data.action:
        raise ValueError('FBX must contain exactly one animated armature')
    donor = rigs[0]
    donor_start, donor_end = donor.animation_data.action.frame_range
    if start < donor_start or end > donor_end:
        raise ValueError(f'Requested {start}–{end} lies outside donor {donor_start}–{donor_end}')
    fps = scene.render.fps / scene.render.fps_base
    if abs(fps - recipe['fps']) > 1e-6:
        raise ValueError(f'Donor FPS {fps} differs from recipe FPS {recipe["fps"]}')
    if any(name not in donor.data.bones for name in bone_map.values()):
        raise ValueError('FBX is missing a mapped bone')
    target.animation_data.action = None
    rest = {bone.name: bone.matrix_local.to_quaternion() for bone in target.data.bones}
    offsets = {}
    for name, donor_name in bone_map.items():
        source_rest = (target.matrix_world.inverted() @ donor.matrix_world @
                       donor.data.bones[donor_name].matrix_local).to_quaternion()
        # Align limb axes first so a T-pose donor does not lower A-pose arms.
        alignment = (rest[name] @ Vector((0, 1, 0))).rotation_difference(source_rest @ Vector((0, 1, 0)))
        offsets[name] = source_rest.inverted() @ alignment @ rest[name]
    samples = []
    for frame in range(start, end + 1):
        scene.frame_set(frame)
        samples.append({name: (target.matrix_world.inverted() @ donor.matrix_world @
                              donor.pose.bones[donor_name].matrix).to_quaternion() @ offsets[name]
                        for name, donor_name in bone_map.items()})
    action = bake_stationary_clip(target, recipe['action'], samples,
                                  recipe['root_bone'], recipe['ground_bones'])
    target.animation_data.action = original_action
    if original_slot:
        target.animation_data.action_slot = original_slot
    for obj in imported:
        bpy.data.objects.remove(obj, do_unlink=True)
    for imported_action in set(bpy.data.actions) - before_actions:
        if imported_action != action:
            bpy.data.actions.remove(imported_action)
    for collection, existing in before_blocks:
        for block in set(collection) - existing:
            if block.users == 0:
                collection.remove(block)
    scene.render.fps, scene.render.fps_base, scene.frame_start, scene.frame_end = original_timing
    scene.frame_set(original_frame)
    for obj in bpy.context.selected_objects:
        obj.select_set(False)
    for obj in original_selection:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = original_active
    bpy.ops.wm.save_as_mainfile(filepath=str(source))
    print(f'Baked {action.name}: donor {start}–{end} -> 1–{len(samples)}, {fps:g} FPS')


if __name__ == '__main__':
    main()
