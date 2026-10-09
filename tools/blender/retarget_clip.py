"""Bake global bone orientations onto an existing stationary character rig."""
import math
import bpy
from mathutils import Quaternion, Vector


def bake_stationary_clip(target, action_name, samples, root_name, ground_names, ground_clearances=None):
    """Replace one action, preserving rig geometry and the supporting foot height."""
    if not samples or any(set(sample) != set(target.data.bones.keys()) for sample in samples):
        raise ValueError('Every sample must cover the complete target rig')
    for name in [root_name, *ground_names]:
        if name not in target.data.bones:
            raise ValueError(f'Unknown grounding bone: {name}')
    if not ground_names or target.data.bones[root_name].parent:
        raise ValueError('Grounding requires foot bones and a parentless root bone')
    if ground_clearances is None:
        ground_clearances = [0.0] * len(samples)
    if len(ground_clearances) != len(samples) or any(not math.isfinite(value) or value < 0 for value in ground_clearances):
        raise ValueError('Ground clearances must be finite, nonnegative and cover every sample')
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
            0, 0, ground_height + ground_clearances[frame - 1] - min(target.pose.bones[name].head.z for name in ground_names)))
        for bone in target.pose.bones:
            bone.keyframe_insert('rotation_quaternion', frame=frame, group=bone.name)
            bone.keyframe_insert('location', frame=frame, group=bone.name)
    if tuple(action.frame_range) != (1, len(samples)):
        raise ValueError('Baked action does not match the requested inclusive range')
    return action


def body_bounds(target, meshes):
    """Bounds of evaluated body geometry in the armature's authoring space."""
    points = []
    graph = bpy.context.evaluated_depsgraph_get()
    inverse = target.matrix_world.inverted()
    for obj in meshes:
        evaluated = obj.evaluated_get(graph)
        mesh = evaluated.to_mesh()
        try:
            transform = inverse @ evaluated.matrix_world
            points.extend(transform @ vertex.co for vertex in mesh.vertices)
        finally:
            evaluated.to_mesh_clear()
    if not points or any(not math.isfinite(value) for point in points for value in point):
        raise ValueError('Body grounding requires finite, nonempty evaluated meshes')
    return ([min(point[axis] for point in points) for axis in range(3)],
            [max(point[axis] for point in points) for axis in range(3)])


def bake_body_clip(target, action_name, samples, root_name, root_offsets, meshes, clearance=0.005):
    """Preserve a falling root trajectory, correcting body penetration vertically."""
    if not samples or len(root_offsets) != len(samples) or any(
            set(sample) != set(target.data.bones.keys()) for sample in samples):
        raise ValueError('Body samples and root offsets must cover the complete clip and rig')
    if root_name not in target.data.bones or target.data.bones[root_name].parent:
        raise ValueError('Body grounding requires a parentless root bone')
    if not math.isfinite(clearance) or clearance < 0 or any(
            not math.isfinite(value) for offset in root_offsets for value in offset):
        raise ValueError('Body clearance and root offsets must be finite')
    if not meshes or any(obj.type != 'MESH' or not any(
            modifier.type == 'ARMATURE' and modifier.object == target for modifier in obj.modifiers)
            for obj in meshes):
        raise ValueError('Body grounding meshes must be skinned to the target rig')
    previous = bpy.data.actions.get(action_name)
    if previous:
        bpy.data.actions.remove(previous)
    action = bpy.data.actions.new(action_name)
    action.use_fake_user = True
    target.animation_data.action = action
    rest = {bone.name: bone.matrix_local.to_quaternion() for bone in target.data.bones}
    root_inverse = rest[root_name].inverted()
    previous_rotations = {}
    vertical_anchor = None
    for frame, (desired, offset) in enumerate(zip(samples, root_offsets), start=1):
        for bone in target.pose.bones:
            bone.matrix_basis.identity()
            parent_rest = rest[bone.parent.name] if bone.parent else Quaternion()
            parent_pose = desired[bone.parent.name] if bone.parent else Quaternion()
            bone.rotation_mode = 'QUATERNION'
            bone.rotation_quaternion = ((parent_rest.inverted() @ rest[bone.name]).inverted() @
                                        parent_pose.inverted() @ desired[bone.name])
            if bone.name in previous_rotations:
                bone.rotation_quaternion.make_compatible(previous_rotations[bone.name])
            previous_rotations[bone.name] = bone.rotation_quaternion.copy()
        root = target.pose.bones[root_name]
        root.location = root_inverse @ offset
        bpy.context.view_layer.update()
        minimum, _ = body_bounds(target, meshes)
        if vertical_anchor is None:
            # The donor may begin with bent knees; anchor its first supporting geometry,
            # rather than the target's taller neutral pelvis, to the floor once.
            vertical_anchor = clearance - minimum[2]
        root.location += root_inverse @ Vector((0, 0, vertical_anchor))
        bpy.context.view_layer.update()
        minimum, _ = body_bounds(target, meshes)
        root.location += root_inverse @ Vector((0, 0, max(0, clearance - minimum[2])))
        bpy.context.view_layer.update()
        for bone in target.pose.bones:
            bone.keyframe_insert('rotation_quaternion', frame=frame, group=bone.name)
            bone.keyframe_insert('location', frame=frame, group=bone.name)
    # Densely sampled motion must not acquire Bezier overshoot between source frames.
    for layer in action.layers:
        for strip in layer.strips:
            for channelbag in strip.channelbags:
                for curve in channelbag.fcurves:
                    for key in curve.keyframe_points:
                        key.interpolation = 'LINEAR'
    if tuple(action.frame_range) != (1, len(samples)):
        raise ValueError('Baked body action does not match its inclusive range')
    return action
