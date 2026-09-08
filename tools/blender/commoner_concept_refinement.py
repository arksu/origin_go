"""Art-directed source-space face, hair and linen refinement for the v3 bake model."""
from __future__ import annotations

import math
import random

import bpy
import numpy as np
from mathutils import Vector

from generate_male_commoner_v3 import bezier, mesh_object, subdivide

TAU = 2 * math.pi


def refine_expression(body, controls):
    basis = body.data.shape_keys.key_blocks[0]
    coordinates = np.empty(len(basis.data) * 3)
    basis.data.foreach_get("co", coordinates)
    coordinates = coordinates.reshape((-1, 3))
    horizontal, depth, height = coordinates.T
    front = np.clip((-depth - .09) / .05, 0, 1)
    delta = np.zeros_like(coordinates)

    # Lift both mouth corners with surrounding cheek tissue, keeping the lips joined.
    smile = np.exp(-((np.abs(horizontal) - .030) / .021) ** 2 - ((height - 1.704) / .025) ** 2) * front
    delta[:, 2] += controls["smile_lift"] * smile
    delta[:, 0] += horizontal * .08 * np.exp(-((height - 1.704) / .027) ** 2 - (horizontal / .062) ** 4) * front
    jaw = np.exp(-((height - 1.671) / .037) ** 2 - ((np.abs(horizontal) - .058) / .042) ** 2)
    delta[:, 0] += np.tanh(horizontal / .025) * controls["jaw_width"] * jaw
    chin = np.exp(-(horizontal / .042) ** 4 - ((height - 1.666) / .028) ** 2) * front
    delta[:, 1] -= .004 * chin
    lower_lids = np.exp(-((np.abs(horizontal) - .037) / .028) ** 4 - ((height - 1.752) / .011) ** 2) * front
    delta[:, 1] -= .002 * lower_lids

    # Broad convex pectorals and the lower arc carry the concept's torso at sprite size.
    chest = np.exp(-((np.abs(horizontal) - .111) / .099) ** 4 - ((height - 1.415) / .065) ** 4)
    chest_front = np.clip((-depth - .035) / .07, 0, 1)
    delta[:, 1] -= controls["chest_projection"] * chest * chest_front
    clavicle_height = 1.534 - .12 * np.abs(horizontal)
    clavicle = np.exp(-((height - clavicle_height) / .015) ** 2 - ((np.abs(horizontal) - .092) / .095) ** 4)
    delta[:, 1] -= .004 * clavicle * chest_front
    key = body.shape_key_add(name="Concept • expression and chest planes", from_mix=False)
    key.data.foreach_set("co", (coordinates + delta).ravel())
    key.value = 1
    bpy.context.view_layer.update()


def hair_lock(name, controls, width, depth, collection):
    vertices, faces, uv_faces = [], [], []
    rows, sides = 32, 16
    skull_center = Vector((0, -.044, 1.796))
    def lock_center(amount):
        point = bezier(controls, amount)
        relative = point - skull_center
        ellipsoid_radius = math.sqrt((relative.x / .102) ** 2 + (relative.y / .121) ** 2 + (relative.z / .147) ** 2)
        contact = skull_center + relative / ellipsoid_radius
        clearance = .001 + .014 * math.sin(math.pi * amount) + .024 * amount ** 5
        if (point - contact).dot(relative.normalized()) > clearance:
            point = contact + relative.normalized() * clearance
        return point
    for row in range(rows + 1):
        amount = row / rows
        center = lock_center(amount)
        tangent = (lock_center(min(1, amount + .002)) - lock_center(max(0, amount - .002))).normalized()
        outward = (center - Vector((0, -.044, 1.79))).normalized()
        lateral = tangent.cross(outward).normalized()
        normal = lateral.cross(tangent).normalized()
        profile = (.40 + .78 * math.sin(math.pi * amount)) * (1 - amount) ** .52 + .008
        for side in range(sides):
            angle = TAU * side / sides
            # One broad crest gives each curl a readable light plane without strand noise.
            crest = 1 + .16 * math.cos(angle * 4)
            point = center + lateral * (math.cos(angle) * width * profile) + normal * (math.sin(angle) * depth * profile * crest)
            vertices.append(tuple(point))
            if row:
                previous = (row - 1) * sides + side
                neighbor = (row - 1) * sides + (side + 1) % sides
                faces.append((previous, neighbor, neighbor + sides, previous + sides))
                uv_faces.append(((side / sides, (row - 1) / rows), ((side + 1) / sides, (row - 1) / rows), ((side + 1) / sides, amount), (side / sides, amount)))
    faces.extend([tuple(reversed(range(sides))), tuple(rows * sides + index for index in range(sides))])
    uv_faces.extend([tuple((0, 0) for _ in range(sides)), tuple((1, 1) for _ in range(sides))])
    obj = mesh_object(name, vertices, faces, collection, uv_faces=uv_faces)
    subdivide(obj, 1)
    return obj


def create_tousled_hair(collection):
    for obj in list(collection.objects):
        bpy.data.objects.remove(obj, do_unlink=True)
    center = Vector((0, -.044, 1.796))
    def scalp(theta, phi, offset=0):
        return center + Vector(((.102 + offset) * math.sin(phi) * math.sin(theta), -(.121 + offset) * math.sin(phi) * math.cos(theta), (.147 + offset) * math.cos(phi)))

    rows, sectors = 24, 80
    vertices = [tuple(scalp(0, 0))]
    faces = []
    for row in range(1, rows + 1):
        for sector in range(sectors):
            theta = TAU * sector / sectors
            limit = 1.66 - .40 * math.cos(theta) + .065 * math.sin(theta * 3)
            vertices.append(tuple(scalp(theta, limit * row / rows)))
            current = 1 + (row - 1) * sectors + sector
            neighbor = 1 + (row - 1) * sectors + (sector + 1) % sectors
            if row == 1:
                faces.append((0, neighbor, current))
            else:
                faces.append((current - sectors, neighbor - sectors, neighbor, current))
    cap = mesh_object("Hair • continuous fitted scalp", vertices, faces, collection)
    subdivide(cap, 1)

    # Side and nape curls flare from embedded roots, instead of forming a combed shell.
    randomizer = random.Random(90421)
    for tier, count in ((0, 13), (1, 15)):
        for index in range(count):
            theta = .82 + (TAU - 1.64) * index / (count - 1)
            theta += randomizer.uniform(-.045, .045)
            phi_start = .33 + .57 * tier + randomizer.uniform(-.08, .08)
            phi_end = 1.31 + .58 * tier - .19 * math.cos(theta) + randomizer.uniform(-.17, .17)
            sweep = .24 * math.sin(theta * 2) + randomizer.uniform(-.13, .13)
            controls = [scalp(theta - .13, phi_start, -.008), scalp(theta - .13, phi_start + .32, .030), scalp(theta + sweep, phi_end - .11, .029), scalp(theta + sweep + .13, phi_end - .02, .036)]
            hair_lock(f"Hair • loose {'crown' if tier == 0 else 'nape'} curl {index:02}", controls, .023 + randomizer.uniform(-.003, .004), .011 + randomizer.uniform(-.001, .002), collection)

    forelocks = [
        ([(-.025,-.110,1.890),(-.002,-.145,1.965),(.045,-.090,1.980),(.008,-.032,1.965)], .023, .014),
        ([(-.013,-.090,1.915),(-.035,-.155,1.967),(-.105,-.152,1.950),(-.123,-.090,1.911)], .024, .014),
        ([(.011,-.049,1.916),(.038,-.077,1.980),(.106,-.077,1.965),(.122,-.022,1.922)], .023, .014),
        ([(.037,-.033,1.904),(.080,-.046,1.975),(.131,-.026,1.947),(.146,.004,1.896)], .024, .014),
        ([(-.035,-.123,1.892),(-.078,-.195,1.928),(-.122,-.174,1.905),(-.121,-.114,1.861)], .026, .014),
        ([(-.019,-.127,1.903),(-.057,-.180,1.934),(-.081,-.195,1.888),(-.052,-.168,1.818)], .025, .013),
        ([(.005,-.113,1.910),(.036,-.189,1.967),(.093,-.177,1.920),(.086,-.137,1.844)], .027, .014),
        ([(.035,-.105,1.897),(.085,-.162,1.926),(.128,-.124,1.878),(.141,-.085,1.840)], .024, .013),
        ([(-.080,-.040,1.843),(-.142,-.099,1.871),(-.135,-.134,1.821),(-.133,-.083,1.777)], .023, .013),
        ([(.085,-.035,1.846),(.140,-.079,1.869),(.132,-.132,1.813),(.132,-.081,1.776)], .022, .012),
    ]
    for index, (controls, width, depth) in enumerate(forelocks):
        hair_lock(f"Hair • swept concept forelock {index:02}", controls, width, depth, collection)


def create_brows(body, collection):
    evaluated = body.evaluated_get(bpy.context.evaluated_depsgraph_get())
    for sign in (-1, 1):
        vertices, faces = [], []
        columns, rows = 40, 6
        for column in range(columns + 1):
            amount = column / columns
            horizontal = sign * (.012 + .058 * amount)
            height = 1.792 + .009 * math.sin(math.pi * amount * .9) - .002 * amount
            half_width = .0045 * math.sin(math.pi * amount) ** .35 * (1 - amount * .65) + .0001
            for row in range(rows + 1):
                vertical = height + half_width * (row / rows * 2 - 1)
                hit, point, normal, _ = evaluated.ray_cast(Vector((horizontal, -1, vertical)), Vector((0, 1, 0)))
                if not hit:
                    raise ValueError("Concept eyebrow missed the face")
                vertices.append(tuple(point + normal * .001))
                if column and row:
                    first = (column - 1) * (rows + 1) + row - 1
                    faces.append((first, first + rows + 1, first + rows + 2, first + 1))
        brow = mesh_object(f"Brow • expressive {'L' if sign > 0 else 'R'}", vertices, faces, collection)
        subdivide(brow, 1)


def create_concept_linen(body, collection):
    for obj in list(collection.objects):
        bpy.data.objects.remove(obj, do_unlink=True)
    evaluated = body.evaluated_get(bpy.context.evaluated_depsgraph_get())
    root_profiles = {}
    root_samples = np.linspace(-.155, .155, 33)
    for back in (False, True):
        sign = 1 if back else -1
        depths = []
        for horizontal in root_samples:
            hit, point, _, _ = evaluated.ray_cast(Vector((horizontal, sign, 1.075)), Vector((0, -sign, 0)))
            if not hit:
                raise ValueError("Cloth panel root missed the waist")
            depths.append(point.y)
        # Cloth follows the broad waist contour, not the body's central skin crease.
        root_profiles[back] = np.polyfit(root_samples, depths, 4)

    def waist(theta, amount):
        height = 1.128 - .072 * amount + .014 * math.sin(theta) ** 2
        outward = Vector((math.sin(theta), -math.cos(theta), 0))
        hit, point, normal, _ = evaluated.ray_cast(Vector((0, 0, height)), outward, distance=.5)
        if not hit:
            raise ValueError("Waistband did not meet the torso")
        fold = .0025 * math.sin(theta * 11 + amount * 1.3) * math.sin(math.pi * amount)
        return point + outward * (.007 + fold + .004 * math.sin(math.pi * amount))

    def panel(horizontal, amount, back):
        sign = 1 if back else -1
        width = .155 * (1 - .13 * amount)
        height = 1.077 - amount * (.247 + .015 * (1 - horizontal * horizontal))
        position_x = width * horizontal
        ray = Vector((0, -sign, 0))
        root_depth = float(np.polyval(root_profiles[back], horizontal * .155))
        smooth_center = math.sqrt(horizontal * horizontal + .035) - math.sqrt(.035)
        fold = .018 * math.cos(math.pi * (smooth_center * 1.5 + amount * 1.6)) * amount ** .6
        fold += .003 * math.sin(horizontal * math.pi * 3 + amount) * amount
        fold += .003 * math.sin(horizontal * math.pi * 5) * math.exp(-amount * 9)
        position_y = root_depth + sign * (.009 + .033 * amount + fold)
        hit, contact, _, _ = evaluated.ray_cast(Vector((position_x, sign, height)), ray)
        if hit:
            desired_depth = sign * position_y
            skin_clearance = sign * contact.y + .008
            # A hard max makes a sharp seam where the drape meets the body envelope.
            # Smooth max stays outside both surfaces and keeps the fabric tangent continuous.
            position_y = sign * (max(desired_depth, skin_clearance) + .006 * math.log1p(math.exp(-abs(desired_depth - skin_clearance) / .006)))
            # The upper edge is tucked under the belt; it must not share its surface.
            transition = min(1, max(0, (amount - .07) / .09))
            transition = transition * transition * (3 - 2 * transition)
            tucked = contact.y + sign * .0025
            position_y = tucked * (1 - transition) + position_y * transition
        return Vector((position_x, position_y, height))

    def surface(name, columns, rows, point, closed=False):
        vertices, faces, uv = [], [], []
        row_width = columns if closed else columns + 1
        for row in range(rows + 1):
            for column in range(row_width):
                vertices.append(tuple(point(column / columns, row / rows)))
                if row and (column or closed):
                    previous = (column - 1) % row_width
                    first = (row - 1) * row_width + previous
                    second = (row - 1) * row_width + column
                    faces.append((first, second, second + row_width, first + row_width))
                    uv.append(((previous / columns, (row - 1) / rows), (column / columns, (row - 1) / rows), (column / columns, row / rows), (previous / columns, row / rows)))
        obj = mesh_object(name, vertices, faces, collection, uv_faces=uv)
        obj["grid_columns"] = row_width
        obj["grid_rows"] = rows + 1
        subdivide(obj, 1)
        thickness = obj.modifiers.new("Linen edge thickness", "SOLIDIFY")
        thickness.thickness = .002
        thickness.offset = 0
        return obj

    surface("Linen • fitted folded waistband", 128, 16, lambda across, down: waist(across * TAU, down), True)
    for back in (False, True):
        name = "rear" if back else "front"
        surface(f"Linen • {name} broad concept panel", 56, 56, lambda across, down: panel(across * 2 - 1, down, back))
        for side in (-1, 1):
            def edge(across, down, side=side, back=back):
                point = panel(side * (1 - .055 * across), down, back)
                point.y += .0015 if back else -.0015
                return point
            surface(f"Linen • {name} flat side hem {side}", 3, 56, edge)
        def bottom(across, down, back=back):
            point = panel(across * 2 - 1, 1 - .025 * down, back)
            point.y += .0015 if back else -.0015
            return point
        surface(f"Linen • {name} flat lower hem", 56, 3, bottom)


def refine_source(body, controls):
    refine_expression(body, controls)
    hair = next(collection for collection in bpy.data.collections if collection.name.startswith("03"))
    cloth = next(collection for collection in bpy.data.collections if collection.name.startswith("04"))
    create_tousled_hair(hair)
    create_brows(body, hair)
    create_concept_linen(body, cloth)
