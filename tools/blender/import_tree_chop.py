"""Bake the supplied Mixamo downward axe swing onto the canonical commoner rig.

Run Blender --background art_source/character/male_commoner/source.blend
--python tools/blender/import_tree_chop.py. Saves only the two new chop actions.
The FBX is a reference input; production exports use the saved source.blend.
"""
import json
from pathlib import Path

import bpy
from mathutils import Matrix, Vector

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'art_source/character/male_commoner/source.blend'
DONOR = SOURCE.parent / 'references/mixamo/standing_melee_attack_downward.fbx'
BONES = {
    'pelvis': 'Hips', 'spine': 'Spine', 'chest': 'Spine2', 'head': 'Head',
    'clavicle.l': 'LeftShoulder', 'upper_arm.l': 'LeftArm',
    'forearm.l': 'LeftForeArm', 'hand.l': 'LeftHand',
    'clavicle.r': 'RightShoulder', 'upper_arm.r': 'RightArm',
    'forearm.r': 'RightForeArm', 'hand.r': 'RightHand',
    'thigh.l': 'LeftUpLeg', 'shin.l': 'LeftLeg', 'foot.l': 'LeftFoot', 'toes.l': 'LeftToeBase',
    'thigh.r': 'RightUpLeg', 'shin.r': 'RightLeg', 'foot.r': 'RightFoot', 'toes.r': 'RightToeBase',
}


def main():
    if Path(bpy.data.filepath).resolve() != SOURCE or not DONOR.is_file():
        raise ValueError('Open the canonical commoner source and provide the declared FBX reference')
    target = bpy.data.objects['commoner_rig']
    if set(BONES) != set(target.data.bones.keys()):
        raise ValueError('Unexpected commoner bone contract')
    scene = bpy.context.scene
    original_frame = scene.frame_current
    original_timing = (scene.render.fps, scene.render.fps_base, scene.frame_start, scene.frame_end)
    original_selection = list(bpy.context.selected_objects)
    original_active = bpy.context.view_layer.objects.active
    original_action = target.animation_data.action
    original_slot = target.animation_data.action_slot
    before_objects = set(bpy.data.objects)
    before_actions = set(bpy.data.actions)
    before_blocks = [(collection, set(collection)) for collection in (bpy.data.meshes, bpy.data.armatures, bpy.data.materials, bpy.data.images)]
    bpy.ops.import_scene.fbx(filepath=str(DONOR))
    imported = set(bpy.data.objects) - before_objects
    rigs = [obj for obj in imported if obj.type == 'ARMATURE']
    if len(rigs) != 1 or not rigs[0].animation_data or not rigs[0].animation_data.action:
        raise ValueError('FBX must contain one animated armature')
    donor = rigs[0]
    donor_names = {name: f'mixamorig:{source}' for name, source in BONES.items()}
    if any(name not in donor.data.bones for name in donor_names.values()):
        raise ValueError('FBX is missing required Mixamo bones')
    start, end = map(int, donor.animation_data.action.frame_range)
    if start != 1 or end <= start:
        raise ValueError('Unexpected donor action range')
    target.animation_data.action = None
    rest = {bone.name: bone.matrix_local.to_quaternion() for bone in target.data.bones}
    offsets = {}
    for name, donor_name in donor_names.items():
        source_rest = (donor.matrix_world @ donor.data.bones[donor_name].matrix_local).to_quaternion()
        # Align the A-pose limb to the donor's T-pose before transferring motion;
        # retaining the entire rest offset would lower every axe swing by ~60 degrees.
        alignment = (rest[name] @ Vector((0, 1, 0))).rotation_difference(source_rest @ Vector((0, 1, 0)))
        offsets[name] = source_rest.inverted() @ alignment @ rest[name]
    rotations = []
    for frame in range(start, end + 1):
        scene.frame_set(frame)
        rotations.append({name: (donor.matrix_world @ donor.pose.bones[donor_name].matrix).to_quaternion() @ offsets[name]
                          for name, donor_name in donor_names.items()})
    mirror = Matrix.Diagonal(Vector((-1, 1, 1)))
    ankle_height = min(target.data.bones[name].head_local.z for name in ('foot.l', 'foot.r'))
    for side in ('r', 'l'):
        name = f'chop_{side}'
        previous = bpy.data.actions.get(name)
        if previous:
            bpy.data.actions.remove(previous)
        action = bpy.data.actions.new(name)
        action.use_fake_user = True
        target.animation_data.action = action
        for index, sample in enumerate(rotations):
            desired = {}
            for bone_name in BONES:
                source_name = bone_name
                if side == 'l':
                    source_name = bone_name[:-1] + ('r' if bone_name.endswith('.l') else 'l') if bone_name.endswith(('.l', '.r')) else bone_name
                rotation = sample[source_name]
                if side == 'l':
                    rotation = (mirror @ rotation.to_matrix() @ mirror).to_quaternion()
                desired[bone_name] = rotation
            # Both exports close at the first pose, avoiding a snap between server cycles.
            if index >= len(rotations) - 7:
                weight = (index - (len(rotations) - 7)) / 6
                for bone_name in desired:
                    first_name = bone_name
                    if side == 'l' and bone_name.endswith(('.l', '.r')):
                        first_name = bone_name[:-1] + ('r' if bone_name.endswith('.l') else 'l')
                    first = rotations[0][first_name]
                    if side == 'l':
                        first = (mirror @ first.to_matrix() @ mirror).to_quaternion()
                    desired[bone_name] = desired[bone_name].slerp(first, weight)
            for bone in target.pose.bones:
                bone.matrix_basis.identity()
                parent_rest = rest[bone.parent.name] if bone.parent else Matrix.Identity(3).to_quaternion()
                parent_pose = desired[bone.parent.name] if bone.parent else Matrix.Identity(3).to_quaternion()
                bone.rotation_mode = 'QUATERNION'
                bone.rotation_quaternion = (parent_rest.inverted() @ rest[bone.name]).inverted() @ parent_pose.inverted() @ desired[bone.name]
            bpy.context.view_layer.update()
            # Keep the supporting foot planted despite the different leg proportions.
            target.pose.bones['pelvis'].location = rest['pelvis'].inverted() @ Vector((0, 0, ankle_height - min(target.pose.bones[name].head.z for name in ('foot.l', 'foot.r'))))
            for bone in target.pose.bones:
                bone.keyframe_insert('rotation_quaternion', frame=index + 1, group=bone.name)
                bone.keyframe_insert('location', frame=index + 1, group=bone.name)
    target.animation_data.action = original_action
    target.animation_data.action_slot = original_slot
    for obj in imported:
        bpy.data.objects.remove(obj, do_unlink=True)
    for action in set(bpy.data.actions) - before_actions:
        if action.name not in {'chop_l', 'chop_r'}:
            bpy.data.actions.remove(action)
    # Imported mesh/material/texture blocks are references, not part of the game asset.
    for collection, existing in before_blocks:
        for block in set(collection) - existing:
            if block.users == 0:
                collection.remove(block)
    scene.frame_set(original_frame)
    scene.render.fps, scene.render.fps_base, scene.frame_start, scene.frame_end = original_timing
    for obj in bpy.context.selected_objects:
        obj.select_set(False)
    for obj in original_selection:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = original_active
    bpy.ops.wm.save_as_mainfile(filepath=str(SOURCE))
    recipe_path = SOURCE.parent / 'asset.yaml'
    recipe = json.loads(recipe_path.read_text())
    for side in ('l', 'r'):
        recipe['clips'][f'chop_{side}'] = {'action': f'chop_{side}', 'range': {'start': 1, 'end': len(rotations)},
                                        'fps': 30, 'loop': True, 'playback': 'time'}
    recipe_path.write_text(json.dumps(recipe, indent=2) + '\n')
    print(f'Baked chop_l/chop_r: {len(rotations)} frames at 30 FPS')


if __name__ == '__main__':
    main()
