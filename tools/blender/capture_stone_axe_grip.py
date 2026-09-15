"""Capture a hand-relative equipment transform from the user's live Blender edit.

Run main() through Blender MCP. Does not regenerate geometry or arm animations.
"""
from pathlib import Path
import json
import math
import bpy
from mathutils import Matrix

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'art_source/equipment/stone_axe'


def main():
    if bpy.context.mode != 'OBJECT':
        raise ValueError('Finish editing and switch to Object Mode before capturing the grip')
    selected = [obj for obj in bpy.context.selected_objects if obj.type == 'MESH' and obj.name.startswith('stone_axe')]
    if len(selected) != 1:
        raise ValueError('Select exactly one stone axe mesh')
    axe = selected[0]
    rig = axe.parent
    if rig is None or rig.type != 'ARMATURE' or axe.parent_type != 'BONE' or axe.parent_bone not in ('hand.l', 'hand.r'):
        raise ValueError('The axe must remain parented to hand.l or hand.r')
    scene = bpy.context.scene
    original_frame, original_subframe = scene.frame_current, scene.frame_subframe
    conversion = Matrix.Rotation(math.pi / 2, 4, 'X')

    def relative():
        bpy.context.view_layer.update()
        hand = rig.matrix_world @ rig.pose.bones[axe.parent_bone].matrix
        return hand.inverted() @ axe.matrix_world @ conversion

    local = relative()
    position, quaternion, scale = local.decompose()
    if max(abs(value - 1) for value in scale) > .00001:
        raise ValueError('Grip capture expects unchanged unit scale; export geometry separately for size edits')
    reconstructed = Matrix.LocRotScale(position, quaternion, scale)
    if max(abs(local[row][column] - reconstructed[row][column]) for row in range(4) for column in range(4)) > .00001:
        raise ValueError('Grip transform contains shear')
    samples = []
    try:
        for frame in range(1, 50, 6):
            scene.frame_set(frame)
            sampled = relative()
            error = max(abs(local[row][column] - sampled[row][column]) for row in range(4) for column in range(4))
            if error > .00001:
                raise ValueError(f'Grip is animated relative to the hand at frame {frame}: {error}')
            samples.append({'frame': frame, 'max_matrix_error': error})
    finally:
        scene.frame_set(original_frame, subframe=original_subframe)
    side = axe.parent_bone[-1]
    transform = {'position': list(position), 'quaternion': [quaternion.x, quaternion.y, quaternion.z, quaternion.w]}
    override_path = OUT / 'grip-overrides.json'
    overrides = json.loads(override_path.read_text()) if override_path.exists() else {}
    overrides[side] = transform
    # Preserve the user's scene separately from the reproducible generated one.
    scene_name = f'stone_axe_user_grip_{side}.blend'
    bpy.data.libraries.write(str(OUT / scene_name), {scene}, fake_user=True, compress=True)
    override_path.write_text(json.dumps(overrides, indent=2)+'\n')
    report_path = OUT / 'export-report.json'
    report = json.loads(report_path.read_text())
    report['bindings'][side] = transform
    report.setdefault('grip_sources', {})[side] = scene_name
    report_path.write_text(json.dumps(report, indent=2)+'\n')
    validation = {'object': axe.name, 'bone': axe.parent_bone, 'authored_frame': original_frame, 'transform': transform, 'samples': samples}
    (OUT / f'user-grip-validation-{side}.json').write_text(json.dumps(validation, indent=2)+'\n')
    return validation
