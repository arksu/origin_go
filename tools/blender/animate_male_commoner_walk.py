"""Build and bake an editable ordinary walk from the approved pixel-style v3."""
from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path

import bpy
import numpy as np
from mathutils import Matrix, Vector
from mathutils.bvhtree import BVHTree

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import adapt_male_commoner_v3_pixel as studio

OUT = studio.OUT / "walk"
JOINTS = {name: Vector(value) for name, value in json.loads((studio.OUT / "landmarks.json").read_text()).items()}
SETTINGS = json.loads((OUT / "walk_config.json").read_text())
if SETTINGS["timeline_frames"] != 48 or SETTINGS["supersampling"] != 4:
    raise ValueError("This walk's sole calibration uses 48 timeline samples and 4x pixel supersampling")
if any(48 % count for count in SETTINGS["sample_counts"]):
    raise ValueError("Export sample counts must divide the 48-frame source cycle")
FOOT_CORRECTIONS = {side: np.zeros(49) for side in ("l", "r")}


def make_rig():
    rig = bpy.data.objects.new("Commoner • walk skeleton", bpy.data.armatures.new("Commoner • deform bones"))
    bpy.context.scene.collection.objects.link(rig)
    bpy.context.view_layer.objects.active = rig
    rig.select_set(True)
    bpy.ops.object.mode_set(mode="EDIT")
    def bone(name, start, end, parent=None):
        result = rig.data.edit_bones.new(name)
        result.head, result.tail = start, end
        if parent:
            result.parent = rig.data.edit_bones[parent]
        return result
    bone("pelvis", JOINTS["pelvis"], JOINTS["spine-4"])
    bone("spine", JOINTS["spine-4"], JOINTS["spine-1"], "pelvis")
    bone("chest", JOINTS["spine-1"], JOINTS["neck"], "spine")
    bone("head", JOINTS["neck"], JOINTS["head-2"], "chest")
    for side in ("l", "r"):
        bone(f"clavicle.{side}", JOINTS[f"{side}-clavicle"], JOINTS[f"{side}-shoulder"], "chest")
        bone(f"upper_arm.{side}", JOINTS[f"{side}-shoulder"], JOINTS[f"{side}-elbow"], f"clavicle.{side}")
        bone(f"forearm.{side}", JOINTS[f"{side}-elbow"], JOINTS[f"{side}-hand"], f"upper_arm.{side}")
        bone(f"hand.{side}", JOINTS[f"{side}-hand"], JOINTS[f"{side}-finger-3-1"], f"forearm.{side}")
        for finger in range(1, 6):
            for segment in range(1, 4):
                bone(f"finger{finger}.{segment}.{side}", JOINTS[f"{side}-finger-{finger}-{segment}"], JOINTS[f"{side}-finger-{finger}-{segment+1}"], f"hand.{side}" if segment == 1 else f"finger{finger}.{segment-1}.{side}")
        ball = JOINTS[f"{side}-foot-1"].copy()
        tip = JOINTS[f"{side}-foot-2"].copy()
        ball.z = tip.z = .040
        bone(f"thigh.{side}", JOINTS[f"{side}-upper-leg"], JOINTS[f"{side}-knee"], "pelvis")
        bone(f"shin.{side}", JOINTS[f"{side}-knee"], JOINTS[f"{side}-ankle"], f"thigh.{side}")
        bone(f"foot.{side}", JOINTS[f"{side}-ankle"], ball, f"shin.{side}")
        bone(f"toes.{side}", ball, tip, f"foot.{side}")
    bpy.ops.object.mode_set(mode="OBJECT")
    rig.show_in_front = True
    rig.data.display_type = "OCTAHEDRAL"
    return rig


def attach_armature(obj, rig):
    modifier = obj.modifiers.new("Walk • preserve muscle volume", "ARMATURE")
    modifier.object = rig
    modifier.use_deform_preserve_volume = True
    return modifier


def bind_body(rig, body, cage):
    """Heat bind the anatomical cage, then barycentrically transfer to the sculpt."""
    cage.hide_set(False)
    for modifier in list(cage.modifiers):
        cage.modifiers.remove(modifier)
    bpy.ops.object.select_all(action="DESELECT")
    cage.select_set(True)
    rig.select_set(True)
    bpy.context.view_layer.objects.active = rig
    bpy.ops.object.parent_set(type="ARMATURE_AUTO")
    names = [bone.name for bone in rig.data.bones]
    if any(name not in cage.vertex_groups for name in names):
        raise RuntimeError("Automatic anatomical cage binding failed")
    weights = np.zeros((len(cage.data.vertices), len(names)))
    for vertex in cage.data.vertices:
        for group in vertex.groups:
            name = cage.vertex_groups[group.group].name
            if name in names:
                weights[vertex.index, names.index(name)] = group.weight
    if np.min(weights.sum(axis=1)) < .5:
        raise RuntimeError("Unweighted vertices in anatomical cage")
    cage.data.calc_loop_triangles()
    triangles = np.array([list(tri.vertices) for tri in cage.data.loop_triangles])
    positions = np.array([list(vertex.co) for vertex in cage.data.vertices])
    tree = BVHTree.FromPolygons(positions.tolist(), triangles.tolist(), all_triangles=True)
    # Shape keys contain the stylized proportions; Mesh.vertices can still hold
    # the original sculpt coordinates. Bind the surface that Blender evaluates.
    evaluated = body.evaluated_get(bpy.context.evaluated_depsgraph_get())
    surface = evaluated.to_mesh()
    if len(surface.vertices) != len(body.data.vertices):
        raise RuntimeError("Skin transfer requires a vertex-preserving rest surface")
    surface_to_cage = cage.matrix_world.inverted() @ body.matrix_world
    nearest = [tree.find_nearest(surface_to_cage @ vertex.co) for vertex in surface.vertices]
    evaluated.to_mesh_clear()
    indices = np.array([hit[2] for hit in nearest])
    points = np.array([tuple(hit[0]) for hit in nearest])
    corners = positions[triangles[indices]]
    edge_a, edge_b, relative = corners[:, 1] - corners[:, 0], corners[:, 2] - corners[:, 0], points - corners[:, 0]
    dot = lambda first, second: np.einsum("ij,ij->i", first, second)
    aa, ab, bb = dot(edge_a, edge_a), dot(edge_a, edge_b), dot(edge_b, edge_b)
    ar, br = dot(edge_a, relative), dot(edge_b, relative)
    denominator = aa * bb - ab * ab
    if np.any(denominator <= 0):
        raise RuntimeError("Degenerate cage triangle in weight transfer")
    bary_b, bary_c = (bb * ar - ab * br) / denominator, (aa * br - ab * ar) / denominator
    bary = np.clip(np.column_stack((1 - bary_b - bary_c, bary_b, bary_c)), 0, 1)
    transferred = np.einsum("ij,ijk->ik", bary, weights[triangles[indices]])
    transferred[transferred < .002] = 0
    transferred /= transferred.sum(axis=1)[:, None]
    for column, name in enumerate(names):
        group = body.vertex_groups.new(name=name)
        # Quantized buckets retain smooth skinning without hundreds of thousands of API calls.
        values = np.rint(transferred[:, column] * 1000).astype(int)
        for value in np.unique(values[values > 0]):
            group.add(np.flatnonzero(values == value).tolist(), value / 1000, "REPLACE")
    attach_armature(body, rig)
    cage.hide_set(True)
    cage.hide_render = True
    print("BOUND", len(body.data.vertices), "vertices to", len(names), "bones", flush=True)


def correct_face_smoothing_group(body):
    """Restrict facial smoothing to the stylized head, rather than old mesh heights."""
    group = body.vertex_groups.get("Face • reduce micro-creases")
    if group is None:
        return
    points = body.data.shape_keys.key_blocks[0].data
    coordinates = np.empty(len(points)*3)
    points.foreach_get("co",coordinates)
    heights = coordinates.reshape((-1,3))[:,2]
    weights = np.rint(studio.smoothstep(1.41,1.46,heights)*1000).astype(int)
    group.remove(list(range(len(points))))
    for value in np.unique(weights[weights>0]):
        group.add(np.flatnonzero(weights==value).tolist(),value/1000,"REPLACE")
    bpy.context.view_layer.update()


def bind_accessories(rig):
    for obj in studio.character_objects():
        if obj.type != "MESH" or obj.name.startswith(("Body", "Anatomical")):
            continue
        group = obj.vertex_groups.new(name="pelvis" if obj.name.startswith("Linen") else "head")
        group.add(list(range(len(obj.data.vertices))), 1, "REPLACE")
        attach_armature(obj, rig)


def transform_at(rotation, location):
    result = rotation.to_4x4()
    result.translation = location
    return result


def aimed_bone(bone, start, end, twist=None):
    rotation = (bone.tail_local - bone.head_local).rotation_difference(end - start).to_matrix()
    if twist is not None:
        rotation = twist @ rotation
    return transform_at(rotation @ bone.matrix_local.to_3x3(), start)


def solve_knee(hip, ankle, upper_length, lower_length):
    axis = ankle - hip
    distance = axis.length
    if distance >= upper_length + lower_length:
        raise ValueError(f"Walk leg exceeds its reach: {distance:.4f}")
    axis.normalize()
    forward = Vector((0, -1, 0))
    bend = (forward - axis * forward.dot(axis)).normalized()
    along = (upper_length**2 - lower_length**2 + distance**2) / (2 * distance)
    return hip + axis * along + bend * math.sqrt(max(0, upper_length**2 - along**2))


def foot_motion(phase, side):
    stance = SETTINGS["stance_fraction"]
    half_step = SETTINGS["half_step_m"]
    if phase <= stance:
        forward = -half_step + 2 * half_step * phase / stance
        lift = 0
        pitch = -12 * (1 - float(studio.smoothstep(0, .10, phase)))
        pitch += 23 * float(studio.smoothstep(.40, stance, phase))
    else:
        amount = (phase - stance) / (1 - stance)
        tangent = 2 * half_step / stance * (1 - stance)
        forward = (2*amount**3-3*amount**2+1)*half_step + (amount**3-2*amount**2+amount)*tangent + (-2*amount**3+3*amount**2)*(-half_step) + (amount**3-amount**2)*tangent
        lift = SETTINGS["foot_lift_m"] * math.sin(math.pi * amount)
        pitch = 23 - 35 * float(studio.smoothstep(0, 1, amount))
    rotation = Matrix.Rotation(math.radians(pitch), 3, "X")
    ankle = JOINTS[f"{side}-ankle"]
    sole = [Vector((0, .064, -.083)), Vector((0, -.158, -.086))]
    height = .003 - min((rotation @ point).z for point in sole) + lift
    height += float(np.interp(phase, np.linspace(0, 1, 49), FOOT_CORRECTIONS[side]))
    pivot = sole[1] if pitch >= 0 else sole[0]
    forward += pivot.y - (rotation @ pivot).y
    sign = 1 if side == "l" else -1
    return Vector((sign * .115, ankle.y + forward, height)), rotation, pitch


def pose_at(rig, phase):
    bones = rig.data.bones
    matrices = {}
    angle = 2 * math.pi * phase
    pelvis_rotation = Matrix.Rotation(-.045 * math.cos(angle), 3, "Z") @ Matrix.Rotation(.035 * math.sin(angle), 3, "Y")
    offset = Vector((.014 * math.sin(angle), 0, -SETTINGS["pelvis_lowering_m"] - SETTINGS["pelvis_bob_m"] * math.cos(2 * angle)))
    pivot = JOINTS["pelvis"]
    pelvis_transform = transform_at(pelvis_rotation, pivot + offset - pelvis_rotation @ pivot)
    matrices["pelvis"] = pelvis_transform @ bones["pelvis"].matrix_local
    spine_start = pelvis_transform @ bones["spine"].head_local
    spine_rotation = Matrix.Rotation(.022 * math.cos(angle), 3, "Z") @ Matrix.Rotation(-.018, 3, "X")
    matrices["spine"] = transform_at(spine_rotation @ bones["spine"].matrix_local.to_3x3(), spine_start)
    spine_transform = matrices["spine"] @ bones["spine"].matrix_local.inverted()
    chest_start = spine_transform @ bones["chest"].head_local
    chest_rotation = Matrix.Rotation(.045 * math.cos(angle), 3, "Z") @ Matrix.Rotation(-.028, 3, "X")
    matrices["chest"] = transform_at(chest_rotation @ bones["chest"].matrix_local.to_3x3(), chest_start)
    chest_transform = matrices["chest"] @ bones["chest"].matrix_local.inverted()
    neck = chest_transform @ bones["head"].head_local
    matrices["head"] = transform_at(Matrix.Rotation(.009 * math.cos(angle), 3, "Z") @ bones["head"].matrix_local.to_3x3(), neck)
    contacts = {}
    for side, sign in (("l", 1), ("r", -1)):
        leg_phase = (phase + (0 if side == "l" else .5)) % 1
        ankle, foot_rotation, pitch = foot_motion(leg_phase, side)
        hip = pelvis_transform @ bones[f"thigh.{side}"].head_local
        knee = solve_knee(hip, ankle, bones[f"thigh.{side}"].length, bones[f"shin.{side}"].length)
        matrices[f"thigh.{side}"] = aimed_bone(bones[f"thigh.{side}"], hip, knee)
        matrices[f"shin.{side}"] = aimed_bone(bones[f"shin.{side}"], knee, ankle)
        matrices[f"foot.{side}"] = transform_at(foot_rotation @ bones[f"foot.{side}"].matrix_local.to_3x3(), ankle)
        ball = ankle + foot_rotation @ (bones[f"toes.{side}"].head_local - bones[f"foot.{side}"].head_local)
        # During push-off the toes stay on the floor while the heel rolls over the ball.
        toe_rotation = Matrix.Identity(3) if pitch > 0 and leg_phase <= SETTINGS["stance_fraction"] else foot_rotation
        matrices[f"toes.{side}"] = transform_at(toe_rotation @ bones[f"toes.{side}"].matrix_local.to_3x3(), ball)
        contacts[side] = {"phase": leg_phase, "stance": leg_phase <= SETTINGS["stance_fraction"], "ankle": list(ankle), "knee": list(knee), "pitch": pitch}
        clavicle = f"clavicle.{side}"
        matrices[clavicle] = chest_transform @ bones[clavicle].matrix_local
        shoulder = chest_transform @ bones[f"upper_arm.{side}"].head_local
        swing = math.cos(angle) * sign
        elbow = shoulder + Vector((sign * .27, .31 * swing, -.96)).normalized() * bones[f"upper_arm.{side}"].length
        wrist = elbow + Vector((sign * .12, .32 * swing - .16, -.96)).normalized() * bones[f"forearm.{side}"].length
        matrices[f"upper_arm.{side}"] = aimed_bone(bones[f"upper_arm.{side}"], shoulder, elbow)
        matrices[f"forearm.{side}"] = aimed_bone(bones[f"forearm.{side}"], elbow, wrist)
        hand_end = wrist + Vector((sign * .07, .20 * swing - .10, -1)).normalized() * bones[f"hand.{side}"].length
        matrices[f"hand.{side}"] = aimed_bone(bones[f"hand.{side}"], wrist, hand_end)
        for finger in range(1, 6):
            for segment in range(1, 4):
                name = f"finger{finger}.{segment}.{side}"
                parent = bones[name].parent.name
                inherited = matrices[parent] @ bones[parent].matrix_local.inverted() @ bones[name].matrix_local
                curl = {1: 30, 2: 45, 3: 28}[segment] if finger > 1 else 9
                across_palm = (JOINTS[f"{side}-finger-5-1"] - JOINTS[f"{side}-finger-2-1"]).normalized()
                local_axis = bones[name].matrix_local.to_3x3().inverted() @ across_palm
                matrices[name] = inherited @ Matrix.Rotation(math.radians(curl) * sign, 4, local_axis)
    for bone in bones:
        desired = matrices[bone.name]
        if bone.parent:
            basis = bone.matrix_local.inverted() @ bone.parent.matrix_local @ matrices[bone.parent.name].inverted() @ desired
        else:
            basis = bone.matrix_local.inverted() @ desired
        rig.pose.bones[bone.name].matrix_basis = basis
    bpy.context.view_layer.update()
    return contacts


def foot_masks(body):
    masks = {}
    for side in ("l", "r"):
        groups = {body.vertex_groups[f"foot.{side}"].index, body.vertex_groups[f"toes.{side}"].index}
        masks[side] = np.array([sum(group.weight for group in vertex.groups if group.group in groups) > .65 for vertex in body.data.vertices])
    return masks


def calibrate_foot_contact(rig, body):
    """Correct against the skinned sole, since enlarged bare feet exceed bone helpers."""
    masks = foot_masks(body)
    for iteration in range(2):
        corrections = {side: FOOT_CORRECTIONS[side].copy() for side in masks}
        for index in range(48):
            phase = index / 48
            pose_at(rig, phase)
            evaluated = body.evaluated_get(bpy.context.evaluated_depsgraph_get())
            coordinates = np.empty(len(body.data.vertices)*3)
            evaluated.data.vertices.foreach_get("co", coordinates)
            coordinates = coordinates.reshape((-1, 3))
            for side, shift in (("l", 0), ("r", 24)):
                sample = (index + shift) % 48
                leg_phase = sample / 48
                lift = 0 if leg_phase <= .60 else SETTINGS["foot_lift_m"] * math.sin(math.pi * (leg_phase-.60)/.40)
                corrections[side][sample] += .001 + lift - float(coordinates[masks[side], 2].min())
        for side in masks:
            corrections[side][48] = corrections[side][0]
            FOOT_CORRECTIONS[side] = corrections[side]
        print("SOLE_CONTACT_CALIBRATED", iteration+1, flush=True)


def key_cycle(rig):
    scene = bpy.context.scene
    scene.render.fps = SETTINGS["timeline_fps"]
    scene.frame_start, scene.frame_end = 1, SETTINGS["timeline_frames"]
    metrics = []
    for index in range(SETTINGS["timeline_frames"] + 1):
        frame = index + 1
        scene.frame_set(frame)
        contacts = pose_at(rig, index / SETTINGS["timeline_frames"])
        metrics.append(contacts)
        for bone in rig.pose.bones:
            bone.rotation_mode = "QUATERNION"
            bone.keyframe_insert("location", frame=frame, group=bone.name)
            bone.keyframe_insert("rotation_quaternion", frame=frame, group=bone.name)
            bone.keyframe_insert("scale", frame=frame, group=bone.name)
    rig.animation_data.action.name = "Walk • ordinary step • 0.96 s • seamless"
    scene.frame_set(1)
    return metrics


def animate_linen(rig, body):
    """Fit broad continuous drapes outside the animated thighs; retain sewn hems."""
    garments = [obj for obj in studio.character_objects() if obj.name.startswith("Linen") and "waistband" not in obj.name]
    bases = {}
    for obj in garments:
        obj.shape_key_add(name="Rest linen")
        bases[obj.name] = np.array([tuple(vertex.co) for vertex in obj.data.vertices])
    # A shared cloth displacement field keeps side and lower seams attached.
    horizontal_samples = np.linspace(-.28, .28, 57)
    height_samples = np.linspace(.55, .96, 48)
    for index in range(SETTINGS["timeline_frames"]):
        frame = index + 1
        bpy.context.scene.frame_set(frame)
        bpy.context.view_layer.update()
        pelvis_transform = rig.pose.bones["pelvis"].matrix @ rig.data.bones["pelvis"].matrix_local.inverted()
        inverse = pelvis_transform.inverted()
        tree = BVHTree.FromObject(body, bpy.context.evaluated_depsgraph_get())
        for side_name, sign in (("front", -1), ("rear", 1)):
            field = np.zeros((len(height_samples), len(horizontal_samples)))
            # Raycast in the pelvis frame so the cloth follows the torso turn.
            for row, height in enumerate(height_samples):
                for column, horizontal in enumerate(horizontal_samples):
                    origin = pelvis_transform @ Vector((horizontal, sign * 1.5, height))
                    ray = pelvis_transform.to_3x3() @ Vector((0, -sign, 0))
                    contact, normal, triangle, distance = tree.ray_cast(origin, ray, 3)
                    if contact is not None:
                        local = inverse @ contact
                        field[row, column] = max(0, sign * local.y + .016)
            # A cloth panel bridges the gap between the thighs instead of sinking
            # into it; a wide smooth envelope also avoids little collision dents.
            for _ in range(5):
                padded = np.pad(field, ((0, 0), (1, 1)), mode="edge")
                field = np.maximum(field, .25*padded[:, :-2] + .50*padded[:, 1:-1] + .25*padded[:, 2:])
            for obj in (item for item in garments if side_name in item.name):
                coordinates = bases[obj.name].copy()
                horizontal, depth, height = coordinates.T.copy()
                columns = np.clip((horizontal-horizontal_samples[0]) / (horizontal_samples[-1]-horizontal_samples[0]) * (len(horizontal_samples)-1), 0, len(horizontal_samples)-1.000001)
                rows = np.clip((height-height_samples[0]) / (height_samples[-1]-height_samples[0]) * (len(height_samples)-1), 0, len(height_samples)-1.000001)
                column_index, row_index = columns.astype(int), rows.astype(int)
                fraction_x, fraction_z = columns-column_index, rows-row_index
                clearance = ((1-fraction_x)*field[row_index,column_index]+fraction_x*field[row_index,column_index+1])*(1-fraction_z)
                clearance += ((1-fraction_x)*field[row_index+1,column_index]+fraction_x*field[row_index+1,column_index+1])*fraction_z
                drape = sign * depth
                blend = 1 - studio.smoothstep(.84, .88, height)
                extra = .003 * math.sin(2 * math.pi * index / SETTINGS["timeline_frames"] - .7) * np.sin(horizontal * 8) * blend
                smooth_max = np.maximum(drape, clearance) + .008 * np.log1p(np.exp(-np.abs(drape-clearance)/.008))
                coordinates[:, 1] = sign * (drape + (smooth_max - drape) * blend + extra)
                key = obj.shape_key_add(name=f"Walk cloth • {frame:02}")
                key.data.foreach_set("co", coordinates.ravel())
                for key_frame, value in ((max(1, frame-1), 0), (frame, 1), (frame+1, 0)):
                    key.value = value
                    key.keyframe_insert("value", frame=key_frame)
                if index == 0:
                    key.value = 0
                    key.keyframe_insert("value", frame=48)
                # Frame 49 equals frame 1, making interpolation across the seam valid.
                key.value = 1 if index == 0 else 0
                key.keyframe_insert("value", frame=49)
                key.value = 0
        if index % 12 == 0:
            print("LINEN_FRAME", frame, flush=True)
    bpy.context.scene.frame_set(1)


def validate_walk(rig, body):
    """Check the actual deformed feet, complete loop seam and fixed-frame contract."""
    scene = bpy.context.scene
    masks = foot_masks(body)
    metrics = []
    for index in range(0, 49, 2):
        scene.frame_set(index+1)
        evaluated = body.evaluated_get(bpy.context.evaluated_depsgraph_get())
        coordinates = np.empty(len(body.data.vertices)*3)
        evaluated.data.vertices.foreach_get("co", coordinates)
        coordinates = coordinates.reshape((-1, 3))
        if not np.isfinite(coordinates).all():
            raise RuntimeError("Non-finite walking surface")
        metric = {"frame": index+1, "feet_min_z": {side: float(coordinates[mask, 2].min()) for side, mask in masks.items()}}
        if min(metric["feet_min_z"].values()) < -.001:
            raise RuntimeError(f"Foot penetrates the ground: {metric}")
        metrics.append(metric)
        if index == 0:
            first = coordinates.copy()
        if index == 48:
            seam_error = float(np.max(np.abs(coordinates-first)))
            if seam_error > 1e-5:
                raise RuntimeError(f"Walk does not loop: {seam_error}")
    (OUT / "validation.json").write_text(json.dumps({"loop_max_vertex_error_m": seam_error, "foot_probes": metrics, "bone_count": len(rig.data.bones), "source_model_unchanged": True}, indent=2)+"\n")
    print("WALK_VALIDATION", seam_error, metrics[:2], flush=True)
    scene.frame_set(1)


def setup_bake(rig):
    scene = bpy.context.scene
    studio.configure_studio()
    scene.render.film_transparent = True
    ground = bpy.data.objects.get("Studio ground")
    if ground:
        ground.hide_render = True
    camera = studio.place_camera("Walk • FIXED orthographic camera", 0, 30, (0, 0, SETTINGS["camera_target_height"]), SETTINGS["camera_scale"])
    actor = bpy.data.objects.new("Walk • MODEL DIRECTION", None)
    scene.collection.objects.link(actor)
    for obj in [rig] + [obj for obj in studio.character_objects() if obj.parent is None]:
        matrix = obj.matrix_world.copy()
        obj.parent = actor
        obj.matrix_world = matrix
    bpy.context.view_layer.update()
    return actor, camera


def save_source():
    scene = bpy.context.scene
    scene.frame_set(1)
    scene.render.resolution_x, scene.render.resolution_y = [dimension * 4 for dimension in SETTINGS["frame_size"]]
    scene.render.filepath = str(OUT / "preview.png")
    scene.cycles.use_animated_seed = False
    bpy.data.objects["Walk • MODEL DIRECTION"].rotation_euler.z = 0
    readme = OUT / "README.md"
    if readme.exists():
        document = bpy.data.texts.get("WALK_README.md") or bpy.data.texts.new("WALK_README.md")
        document.clear()
        document.write(readme.read_text())
    bpy.context.preferences.filepaths.save_version = 0
    bpy.ops.wm.save_as_mainfile(filepath=str(OUT / "male_commoner_walk.blend"), compress=True)


def render_samples(actor, counts, directions):
    scene = bpy.context.scene
    width, height = SETTINGS["frame_size"]
    fixed = {obj.name: obj.matrix_world.copy() for obj in bpy.data.objects if obj.type == "LIGHT" and not obj.hide_render or obj == scene.camera}
    for direction in directions:
        actor.rotation_euler.z = math.radians(studio.DIRECTIONS[direction])
        for count in counts:
            for index in range(count):
                scene.frame_set(1 + index * SETTINGS["timeline_frames"] // count)
                bpy.context.view_layer.update()
                for name, expected in fixed.items():
                    if max(abs(value) for row in (bpy.data.objects[name].matrix_world - expected) for value in row) > 1e-6:
                        raise RuntimeError(f"Camera/light moved during walk bake: {name}")
                path = OUT / "raw" / str(count) / direction / f"{index:02}.png"
                studio.render(path, (width * 4, height * 4), 32)
                print("WALK_FRAME", direction, count, index, flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--inspect", action="store_true")
    parser.add_argument("--preview", action="store_true")
    parser.add_argument("--render-only", action="store_true")
    parser.add_argument("--finalize", action="store_true")
    args = parser.parse_args(sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else [])
    OUT.mkdir(parents=True, exist_ok=True)
    if args.render_only or args.finalize:
        bpy.ops.wm.open_mainfile(filepath=str(OUT / "male_commoner_walk.blend"))
        if args.finalize:
            save_source()
            return
        render_samples(bpy.data.objects["Walk • MODEL DIRECTION"], SETTINGS["sample_counts"], studio.DIRECTIONS)
        return
    bpy.ops.wm.open_mainfile(filepath=str(studio.OUT / "male_commoner_v3_pixel.blend"))
    if args.inspect:
        for obj in studio.character_objects():
            print("OBJECT", obj.name, obj.type, len(obj.data.vertices) if obj.type == "MESH" else "", obj.hide_render, [(mod.name, mod.type) for mod in obj.modifiers], list(obj.vertex_groups.keys())[:8] if obj.type == "MESH" else "", flush=True)
        return
    rig = make_rig()
    body = next(obj for obj in studio.character_objects() if obj.name.startswith("Body"))
    cage = next(obj for obj in studio.character_objects() if obj.name.startswith("Anatomical"))
    correct_face_smoothing_group(body)
    bind_body(rig, body, cage)
    bind_accessories(rig)
    calibrate_foot_contact(rig, body)
    metrics = key_cycle(rig)
    animate_linen(rig, body)
    validate_walk(rig, body)
    actor, camera = setup_bake(rig)
    save_source()
    (OUT / "gait_metrics.json").write_text(json.dumps(metrics, indent=2) + "\n")
    render_samples(actor, [8] if args.preview else SETTINGS["sample_counts"], ["s", "w", "e", "se"] if args.preview else studio.DIRECTIONS)
    print("WALK_COMPLETE", flush=True)


if __name__ == "__main__":
    main()
