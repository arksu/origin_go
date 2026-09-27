"""Bake global bone orientations onto an existing stationary character rig."""
import bpy
from mathutils import Quaternion, Vector


def bake_stationary_clip(target, action_name, samples, root_name, ground_names):
    """Replace one action, preserving rig geometry and the supporting foot height."""
    if not samples or any(set(sample) != set(target.data.bones.keys()) for sample in samples):
        raise ValueError('Every sample must cover the complete target rig')
    for name in [root_name, *ground_names]:
        if name not in target.data.bones:
            raise ValueError(f'Unknown grounding bone: {name}')
    if not ground_names or target.data.bones[root_name].parent:
        raise ValueError('Grounding requires foot bones and a parentless root bone')
    previous = bpy.data.actions.get(action_name)
    if previous:
        bpy.data.actions.remove(previous)
    action = bpy.data.actions.new(action_name)
    action.use_fake_user = True
    target.animation_data.action = action
    rest = {bone.name: bone.matrix_local.to_quaternion() for bone in target.data.bones}
    ground_height = min(target.data.bones[name].head_local.z for name in ground_names)
    for frame, desired in enumerate(samples, start=1):
        for bone in target.pose.bones:
            bone.matrix_basis.identity()
            parent_rest = rest[bone.parent.name] if bone.parent else Quaternion()
            parent_pose = desired[bone.parent.name] if bone.parent else Quaternion()
            bone.rotation_mode = 'QUATERNION'
            bone.rotation_quaternion = ((parent_rest.inverted() @ rest[bone.name]).inverted() @
                                        parent_pose.inverted() @ desired[bone.name])
        bpy.context.view_layer.update()
        # Different limb lengths must not make a stationary action float above the ground.
        target.pose.bones[root_name].location = rest[root_name].inverted() @ Vector((
            0, 0, ground_height - min(target.pose.bones[name].head.z for name in ground_names)))
        for bone in target.pose.bones:
            bone.keyframe_insert('rotation_quaternion', frame=frame, group=bone.name)
            bone.keyframe_insert('location', frame=frame, group=bone.name)
    if tuple(action.frame_range) != (1, len(samples)):
        raise ValueError('Baked action does not match the requested inclusive range')
    return action
