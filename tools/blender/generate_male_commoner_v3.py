"""Build the editable v3 character from a pinned anatomical control mesh.

Blender --background --python tools/blender/generate_male_commoner_v3.py -- --preview
The upstream CC0 assets and all art controls are local; no network is used.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import random
import sys
from pathlib import Path

import bpy
import bmesh
import numpy as np
from mathutils import Vector

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "art_source/characters/male_commoner_v3"
SOURCE = OUT / "source/makehuman"
SYSTEM = OUT / "source/system_assets"
CONFIG = json.loads((OUT / "model_config.json").read_text())
PREVIEWS = OUT / "previews"
TAU = 2 * math.pi


def verify_sources():
    for source in (SOURCE,SYSTEM):
        manifest = json.loads((source / "manifest.json").read_text())
        for name, checksum in manifest["files"].items():
            path = source / name
            if not path.exists() or hashlib.sha256(path.read_bytes()).hexdigest() != checksum:
                raise ValueError(f"Missing or changed source asset: {path}")


def read_obj(path):
    vertices, texcoords, faces, groups = [], [], [], {}
    group = "default"
    for line in path.read_text().splitlines():
        tokens = line.split()
        if not tokens:
            continue
        if tokens[0] == "v":
            vertices.append(tuple(map(float, tokens[1:4])))
        elif tokens[0] == "vt":
            texcoords.append(tuple(map(float, tokens[1:3])))
        elif tokens[0] == "g":
            group = tokens[1]
        elif tokens[0] == "f":
            corners = [tuple(int(number) - 1 if number else -1 for number in value.split("/")) for value in tokens[1:]]
            faces.append((group, corners))
            groups.setdefault(group, set()).update(corner[0] for corner in corners)
    return np.array(vertices, dtype=float), texcoords, faces, groups


def make_collection(name):
    coll = bpy.data.collections.new(name)
    bpy.context.scene.collection.children.link(coll)
    return coll


def move_to(obj, coll):
    for current in list(obj.users_collection):
        current.objects.unlink(obj)
    coll.objects.link(obj)


def mesh_object(name, vertices, faces, coll, material=None, uv_faces=None):
    mesh = bpy.data.meshes.new(name + "_mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.update()
    obj = bpy.data.objects.new(name, mesh)
    coll.objects.link(obj)
    if material:
        mesh.materials.append(material)
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    if uv_faces:
        uv = mesh.uv_layers.new(name="UVMap")
        for polygon, coords in zip(mesh.polygons, uv_faces):
            for index, coord in zip(polygon.loop_indices, coords):
                uv.data[index].uv = coord
    editable=bmesh.new()
    editable.from_mesh(mesh)
    bmesh.ops.recalc_face_normals(editable,faces=list(editable.faces))
    editable.to_mesh(mesh)
    editable.free()
    return obj


def subdivide(obj, level=2):
    modifier = obj.modifiers.new("Surface refinement", "SUBSURF")
    modifier.levels = min(level, 2)
    modifier.render_levels = level
    return modifier


def principled(name, color, roughness=.5):
    material = bpy.data.materials.new(name)
    material.use_nodes = True
    material.diffuse_color = (*color, 1)
    shader = material.node_tree.nodes.get("Principled BSDF")
    shader.inputs["Base Color"].default_value = (*color, 1)
    shader.inputs["Roughness"].default_value = roughness
    return material, shader


def skin_material():
    mat, shader = principled("Skin • warm ochre / soft subsurface", (.49, .245, .118), .57)
    nodes, links = mat.node_tree.nodes, mat.node_tree.links
    shader.inputs["Subsurface Weight"].default_value = .095
    shader.inputs["Subsurface Radius"].default_value = (1.0, .42, .20)
    shader.inputs["Subsurface Scale"].default_value = .014
    shader.inputs["Specular IOR Level"].default_value = .24
    coord = nodes.new("ShaderNodeTexCoord")
    broad = nodes.new("ShaderNodeTexNoise")
    broad.inputs["Scale"].default_value = 8
    broad.inputs["Detail"].default_value = 3
    links.new(coord.outputs["Generated"], broad.inputs["Vector"])
    ramp = nodes.new("ShaderNodeValToRGB")
    ramp.color_ramp.elements[0].position = .15
    ramp.color_ramp.elements[0].color = (.39, .172, .073, 1)
    ramp.color_ramp.elements[1].position = .85
    ramp.color_ramp.elements[1].color = (.54, .288, .143, 1)
    links.new(broad.outputs["Fac"], ramp.inputs["Fac"])
    attr = nodes.new("ShaderNodeVertexColor")
    attr.layer_name = "SkinTint"
    mix = nodes.new("ShaderNodeMixRGB")
    mix.blend_type = "MULTIPLY"
    mix.inputs[0].default_value = .62
    links.new(ramp.outputs["Color"], mix.inputs[1])
    links.new(attr.outputs["Color"], mix.inputs[2])
    image=bpy.data.images.load(str(SYSTEM/"skins/young_caucasian_male/young_lightskinned_male_diffuse.png"))
    image.pack()
    texture=nodes.new("ShaderNodeTexImage")
    texture.image=image
    painted=nodes.new("ShaderNodeMixRGB")
    painted.inputs[0].default_value=.50
    links.new(mix.outputs["Color"],painted.inputs[1])
    links.new(texture.outputs["Color"],painted.inputs[2])
    warmth=nodes.new("ShaderNodeMixRGB")
    warmth.blend_type="MULTIPLY"
    warmth.inputs[0].default_value=1
    warmth.inputs[2].default_value=(.86,.74,.60,1)
    links.new(painted.outputs["Color"],warmth.inputs[1])
    links.new(warmth.outputs["Color"], shader.inputs["Base Color"])
    pore = nodes.new("ShaderNodeTexNoise")
    pore.inputs["Scale"].default_value = 1200
    pore.inputs["Detail"].default_value = 2
    links.new(coord.outputs["Object"], pore.inputs["Vector"])
    bump = nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value = .20
    bump.inputs["Distance"].default_value = .00022
    links.new(pore.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    return mat


def anatomical_source():
    base, uv, faces, groups = read_obj(SOURCE / "base.obj")
    coords = base.copy()
    for name, weight in CONFIG["target_weights"].items():
        if not math.isfinite(weight):
            raise ValueError(f"Invalid target weight: {name}")
        for line in (SOURCE / "targets" / (name + ".target")).read_text().splitlines():
            if not line or line.startswith("#"):
                continue
            index, *delta = line.split()
            coords[int(index)] += np.array(delta, dtype=float) * weight
    body_indices = sorted(groups["body"])
    ground = float(coords[body_indices, 1].min())
    scale = CONFIG["height_m"] / (float(coords[body_indices, 1].max()) - ground)

    def convert(points):
        result = np.column_stack((points[:, 0] * scale, -points[:, 2] * scale, (points[:, 1] - ground) * scale))
        influence = np.clip((result[:, 2] - 1.59) / .095, 0, 1)
        influence = influence * influence * (3 - 2 * influence)
        result += (result - np.array((0,-.052,1.69))) * influence[:,None] * (CONFIG.get("head_scale",1) - 1)
        return result

    converted = convert(coords)
    joints = {name.removeprefix("joint-"): converted[sorted(indices)].mean(axis=0) for name, indices in groups.items() if name.startswith("joint-")}
    return coords, converted, uv, faces, groups, joints, convert


def create_body(converted, texcoords, faces, groups, coll, skin):
    indices = sorted(groups["body"])
    mapping = {old: new for new, old in enumerate(indices)}
    body_faces = [corners for name, corners in faces if name == "body"]
    obj = mesh_object("Body • continuous anatomical surface", converted[indices].tolist(),
                      [[mapping[corner[0]] for corner in face] for face in body_faces], coll, skin,
                      [[texcoords[corner[1]] for corner in face] for face in body_faces])
    obj["source"] = "MakeHuman hm08 CC0; see source/makehuman/manifest.json"
    obj["stage"] = "Static character source; no animation or skinning claimed"
    original = obj.data.attributes.new("source_vertex_id", "INT", "POINT")
    original.data.foreach_set("value", indices)
    tint = obj.data.color_attributes.new(name="SkinTint", type="FLOAT_COLOR", domain="POINT")
    # Local colour is restrained so the model remains legible under new light.
    for index, coord in enumerate(converted[indices]):
        x, y, z = coord
        color = np.ones(3)
        lip = math.exp(-((z - 1.704)/.010)**2 - (x/.033)**6) * max(0, min(1, (-y - .145)/.025))
        cheek = math.exp(-((abs(x) - .051)/.027)**2 - ((z - 1.727)/.030)**2) * max(0, min(1, (-y - .04)/.04))
        ear = math.exp(-((abs(x) - .090)/.018)**2 - ((z - 1.742)/.040)**2)
        redness = min(.48, lip*.48 + cheek*.13 + ear*.18)
        color *= (1., 1 - redness, 1 - redness*.76)
        tint.data[index].color = (*color, 1)
    subdivide(obj, 2)
    return obj


def sculpt_anatomy(body, joints, source_coll):
    cage = body.copy()
    cage.data = body.data.copy()
    cage.name = "Anatomical control cage • UV / original vertex IDs"
    source_coll.objects.link(cage)
    cage.hide_render = True
    cage.hide_set(True)
    bpy.context.view_layer.objects.active = body
    body.select_set(True)
    bpy.ops.object.modifier_apply(modifier=body.modifiers[0].name)
    body.shape_key_add(name="Anatomical surface")
    detail = body.shape_key_add(name="Concept • muscular surface sculpt")
    count = len(body.data.vertices)
    coords = np.empty(count*3)
    normals = np.empty(count*3)
    body.data.vertices.foreach_get("co",coords)
    body.data.vertices.foreach_get("normal",normals)
    coords, normals = coords.reshape((-1,3)), normals.reshape((-1,3))
    x,y,z = coords.T
    absolute_x = np.abs(x)
    front = np.clip(-normals[:,1],0,1)**1.5
    back = np.clip(normals[:,1],0,1)**1.4
    displacement = np.zeros(count)

    # Sculpt continuous skin; grooves and muscle bellies share the same mesh.
    for height,width,height_radius,amount in [(1.295,.046,.036,.012),(1.216,.045,.035,.013),(1.140,.043,.034,.011)]:
        displacement += amount*np.exp(-((absolute_x-.049)/width)**2-((z-height)/height_radius)**4)*front
    displacement -= .002*np.exp(-(x/.010)**2-((z-1.225)/.14)**6)*front
    chest_height=1.353 + .09*(absolute_x-.09)
    displacement += .018*np.exp(-((absolute_x-.118)/.100)**4-((z-1.410)/.061)**4)*front
    displacement -= .0045*np.exp(-((z-chest_height)/.012)**2-((absolute_x-.13)/.100)**6)*front
    displacement -= .002*np.exp(-(x/.012)**2-((z-1.433)/.08)**4)*front
    for row in range(3):
        rib_z = 1.315 - row*.031 - .48*(absolute_x-.145)
        displacement += .0035*np.exp(-((z-rib_z)/.015)**2-((absolute_x-.16)/.047)**4)*front
    displacement += .008*np.exp(-((absolute_x-.12)/.032)**2-((z-1.18)/.12)**4)*front
    displacement -= .004*np.exp(-((z-(1.07+.55*absolute_x))/.013)**2-((absolute_x-.10)/.10)**4)*front
    displacement += .012*np.exp(-((absolute_x-.052)/.038)**2-((z-1.31)/.21)**4)*back
    displacement -= .005*np.exp(-(x/.018)**2-((z-1.36)/.23)**4)*back
    displacement += .011*np.exp(-((absolute_x-.15)/.07)**2-((z-1.40)/.11)**2)*back
    trapezius_height=1.568-.68*absolute_x
    displacement += .009*np.exp(-((z-trapezius_height)/.042)**2-((absolute_x-.085)/.10)**4)*back
    scapula_height=1.448-.30*(absolute_x-.10)
    displacement += .009*np.exp(-((z-scapula_height)/.029)**2-((absolute_x-.12)/.06)**4)*back
    displacement -= .0035*np.exp(-((z-(scapula_height-.055))/.018)**2-((absolute_x-.135)/.060)**4)*back
    lat_center=.12+.32*(z-1.28)
    displacement += .013*np.exp(-((absolute_x-lat_center)/.055)**2-((z-1.34)/.12)**4)*back

    def limb_region(start,end):
        axis = np.array(end)-np.array(start)
        length=np.linalg.norm(axis)
        direction=axis/length
        local=coords-np.array(start)
        along=(local@direction)/length
        radial=local-along[:,None]*axis
        distance=np.linalg.norm(radial,axis=1)
        return along, radial, distance

    for side in ("l","r"):
        shoulder,elbow,hand=[joints[f"{side}-{name}"] for name in ("shoulder","elbow","hand")]
        along,radial,distance=limb_region(shoulder,elbow)
        envelope=np.exp(-((along-.53)/.27)**4)*np.exp(-(distance/.145)**8)
        displacement += .012*envelope*np.clip(-normals[:,1],0,1)
        displacement += .015*envelope*np.clip(normals[:,1],0,1)
        delta=coords-shoulder
        shoulder_region=np.exp(-np.sum((delta/np.array((.10,.14,.105)))**2,axis=1))
        displacement += .008*shoulder_region
        along,radial,distance=limb_region(elbow,hand)
        envelope=np.exp(-((along-.32)/.29)**4)*np.exp(-(distance/.095)**8)
        displacement += .011*envelope
        knee,ankle=joints[f"{side}-knee"],joints[f"{side}-ankle"]
        thigh_start=np.array((knee[0]*.65,.015,1.02))
        along,radial,distance=limb_region(thigh_start,knee)
        envelope=np.exp(-((along-.40)/.40)**4)*np.exp(-(distance/.155)**8)
        displacement += .018*envelope*front
        displacement += .010*np.exp(-((along-.81)/.14)**2)*np.exp(-(distance/.14)**8)*front
        along,radial,distance=limb_region(knee,ankle)
        envelope=np.exp(-((along-.35)/.25)**4)*np.exp(-(distance/.11)**8)
        displacement += .012*envelope*back
        wrist=Vector(joints[f"{side}-hand"])
        hand_axis=(Vector(joints[f"{side}-finger-3-1"])-wrist).normalized()
        across=(Vector(joints[f"{side}-finger-2-1"])-Vector(joints[f"{side}-finger-5-1"])).normalized()
        dorsal=across.cross(hand_axis).normalized()
        if dorsal.z<0:dorsal.negate()
        dorsal_mask=np.clip(normals@np.array(dorsal),0,1)**2
        for digit in range(1,6):
            digit_start=np.array(joints[f"{side}-finger-{digit}-1"])
            digit_end=np.array(joints[f"{side}-finger-{digit}-4"])
            axis=(digit_end-digit_start)/np.linalg.norm(digit_end-digit_start)
            for joint_index in (2,3):
                joint=np.array(joints[f"{side}-finger-{digit}-{joint_index}"])
                relative=coords-joint
                along_joint=relative@axis
                mask=np.exp(-np.sum(relative**2,axis=1)/.018**2)*dorsal_mask
                displacement -= .00055*mask*(np.exp(-(along_joint/.0012)**2)+.5*np.exp(-((along_joint-.0024)/.0008)**2))
            if digit>1:
                along,radial,distance=limb_region(wrist.lerp(Vector(digit_start),.25),Vector(digit_start))
                displacement += .0014*np.exp(-((along-.55)/.47)**6)*np.exp(-(distance/.009)**2)*dorsal_mask
    detail.data.foreach_set("co",(coords+normals*displacement[:,None]).ravel())
    detail.value=1
    body["sculpt_method"]="Surface displacement on subdivided quad topology; no voxel remeshing"
    return cage


def fit_proxy(path,raw):
    scale = np.ones(3)
    mapped, reading = [], False
    for line in path.read_text().splitlines():
        tokens = line.split()
        if not tokens or tokens[0].startswith("#"):
            continue
        if tokens[0] in ("x_scale", "y_scale", "z_scale"):
            axis = "xyz".index(tokens[0][0])
            first, second = map(int, tokens[1:3])
            scale[axis] = abs(raw[first, axis] - raw[second, axis]) / float(tokens[3])
        elif tokens[0] == "verts":
            reading = True
        elif reading and len(tokens) == 9:
            ids = list(map(int, tokens[:3]))
            weights = np.array(tokens[3:6], dtype=float)
            mapped.append((raw[ids] * weights[:, None]).sum(axis=0) + np.array(tokens[6:9], dtype=float) * scale)
        elif reading and len(tokens)==1 and tokens[0].isdigit():
            mapped.append(raw[int(tokens[0])])
        elif reading:
            break
    return np.array(mapped)


def create_eyes(raw, convert, coll):
    _, texcoords, faces, _ = read_obj(SOURCE / "eyes/high-poly/high-poly.obj")
    mapped=fit_proxy(SOURCE / "eyes/high-poly/high-poly.mhclo",raw)
    material, shader = principled("Eyes • chestnut iris / wet sclera", (.8,.8,.7), .18)
    image = bpy.data.images.load(str(SOURCE / "eyes/materials/brown_eye.png"))
    image.pack()
    texture = material.node_tree.nodes.new("ShaderNodeTexImage")
    texture.image = image
    material.node_tree.links.new(texture.outputs["Color"], shader.inputs["Base Color"])
    shader.inputs["Coat Weight"].default_value = .35
    shader.inputs["Coat Roughness"].default_value = .08
    obj = mesh_object("Eyes • fitted iris and cornea", convert(np.array(mapped)).tolist(),
                      [[corner[0] for corner in corners] for _, corners in faces], coll, material,
                      [[texcoords[corner[1]] for corner in corners] for _, corners in faces])
    subdivide(obj, 1)
    cornea, cornea_shader = principled("Cornea • clear optical shell",(1,1,1),.025)
    cornea_shader.inputs["Transmission Weight"].default_value=1
    cornea_shader.inputs["IOR"].default_value=1.376
    obj.data.materials.append(cornea)
    # The upstream eye has separate optical shells with a dummy UV island.
    for polygon in obj.data.polygons:
        if all(index<256 or 532<=index<788 for index in polygon.vertices):
            polygon.material_index=1
    return obj


def curve_object(name, points, radius, coll, material, cyclic=False):
    curve=bpy.data.curves.new(name,"CURVE")
    curve.dimensions="3D"
    curve.resolution_u=12
    curve.bevel_depth=radius
    curve.bevel_resolution=3
    spline=curve.splines.new("POLY")
    spline.points.add(len(points)-1)
    for point,coord in zip(spline.points,points):
        point.co=(*coord,1)
    spline.use_cyclic_u=cyclic
    obj=bpy.data.objects.new(name,curve)
    coll.objects.link(obj)
    curve.materials.append(material)
    return obj


def bezier(controls,t):
    a,b,c,d=[Vector(point) for point in controls]
    return (1-t)**3*a+3*(1-t)**2*t*b+3*(1-t)*t*t*c+t**3*d


def hair_material(name,color):
    mat,shader=principled(name,color,.57)
    shader.inputs["Anisotropic"].default_value=.25
    shader.inputs["Specular IOR Level"].default_value=.24
    shader.inputs["Coat Weight"].default_value=.025
    shader.inputs["Coat Roughness"].default_value=.35
    nodes,links=mat.node_tree.nodes,mat.node_tree.links
    uv=nodes.new("ShaderNodeTexCoord")
    stretch=nodes.new("ShaderNodeVectorMath")
    stretch.operation="MULTIPLY"
    stretch.inputs[1].default_value=(28,1.5,1)
    links.new(uv.outputs["UV"],stretch.inputs[0])
    noise=nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value=5
    noise.inputs["Detail"].default_value=2
    links.new(stretch.outputs["Vector"],noise.inputs["Vector"])
    ramp=nodes.new("ShaderNodeValToRGB")
    ramp.color_ramp.elements[0].color=(*[v*.55 for v in color],1)
    ramp.color_ramp.elements[1].color=(*[v*1.05 for v in color],1)
    links.new(noise.outputs["Fac"],ramp.inputs["Fac"])
    links.new(ramp.outputs["Color"],shader.inputs["Base Color"])
    bump=nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value=.18
    bump.inputs["Distance"].default_value=.00035
    links.new(noise.outputs["Fac"],bump.inputs["Height"])
    links.new(bump.outputs["Normal"],shader.inputs["Normal"])
    return mat


def sculpted_lock(name,controls,width,depth,coll,material):
    rows,sides=30,20
    vertices,faces,uv=[],[],[]
    for row in range(rows+1):
        t=row/rows
        center=bezier(controls,t)
        tangent=(bezier(controls,min(1,t+.003))-bezier(controls,max(0,t-.003))).normalized()
        outward=(center-Vector((0,-.045,1.79))).normalized()
        lateral=tangent.cross(outward).normalized()
        normal=lateral.cross(tangent).normalized()
        profile=(.48+.70*math.sin(math.pi*t*.88))*(1-t)**.48+.012
        for side in range(sides):
            angle=TAU*side/sides
            ridge=1+.065*math.cos(angle*7+.35*math.sin(t*4))
            position=center+lateral*(math.cos(angle)*width*profile)+normal*(math.sin(angle)*depth*profile*ridge)
            vertices.append(tuple(position))
        if row:
            for side in range(sides):
                first=(row-1)*sides+side
                second=(row-1)*sides+(side+1)%sides
                faces.append((first,second,second+sides,first+sides))
                uv.append(((side/sides,(row-1)/rows),((side+1)/sides,(row-1)/rows),((side+1)/sides,t),(side/sides,t)))
    faces.extend([tuple(reversed(range(sides))),tuple(rows*sides+i for i in range(sides))])
    uv.extend([tuple((0,0) for _ in range(sides)),tuple((1,1) for _ in range(sides))])
    obj=mesh_object(name,vertices,faces,coll,material,uv)
    subdivide(obj,1)
    return obj


def create_hair(coll,body):
    palette=[hair_material("Chestnut • "+label,color) for label,color in [
        ("umber",(.083,.029,.011)),("warm brown",(.10,.040,.016)),
        ("golden edge",(.125,.052,.018)),("deep root",(.072,.025,.010))]]
    # A fitted scalp surface closes the spaces between overlapping swept locks.
    center=Vector((0,-.054,1.794))
    def scalp(theta,phi,offset=0):
        return center+Vector(((.099+offset)*math.sin(phi)*math.sin(theta),-(.119+offset)*math.sin(phi)*math.cos(theta),(.145+offset)*math.cos(phi)))
    vertices,faces=[],[]
    rings,sectors=24,80
    for ring in range(rings+1):
        for sector in range(sectors):
            theta=TAU*sector/sectors
            front_weight=max(0,math.cos(theta))**4
            limit=1.68-.40*math.cos(theta)+.04*math.sin(theta*3)+front_weight*(.075*math.sin(theta*9)+.045*math.cos(theta*13))
            vertices.append(tuple(scalp(theta,.012+(limit-.012)*ring/rings)))
            if ring:
                a=(ring-1)*sectors+sector;b=(ring-1)*sectors+(sector+1)%sectors
                faces.append((a,b,b+sectors,a+sectors))
    cap=mesh_object("Hair • fitted underlayer",vertices,faces,coll,palette[0])
    subdivide(cap,1)
    randomizer=random.Random(8304)
    for tier,(phi_start,phi_end,count) in enumerate([(.20,1.27,22),(.82,1.86,25)]):
        for index in range(count):
            theta=TAU*index/count+.10*tier
            # Leave the forehead for the individually art-directed forelocks.
            if abs(math.atan2(math.sin(theta),math.cos(theta)))<.85:
                continue
            end=phi_end-.30*math.cos(theta)+randomizer.uniform(-.12,.12)
            start=phi_start-.08*math.cos(theta)+randomizer.uniform(-.06,.06)
            sweep=.28+.10*math.sin(theta)+randomizer.uniform(-.12,.12)
            controls=[scalp(theta-sweep,start,-.010),scalp(theta-.13,start+.30,.012),scalp(theta+.10,end-.2,.012),scalp(theta+.18,end,.003)]
            sculpted_lock(f"Hair • layered sweep {tier+1}.{index+1:02}",controls,.018+randomizer.random()*.014,.006+randomizer.random()*.003,coll,palette[index%3])
    forelocks=[
        ([(-.07,-.06,1.885),(-.075,-.10,1.971),(-.035,-.16,1.990),(.037,-.16,1.943)],.026,.016),
        ([(-.053,-.075,1.899),(-.065,-.13,1.988),(.005,-.18,1.978),(.065,-.148,1.902)],.026,.017),
        ([(-.030,-.074,1.911),(-.028,-.13,1.992),(.050,-.16,1.975),(.084,-.113,1.88)],.023,.015),
        ([(-.004,-.056,1.916),(.005,-.12,1.991),(.073,-.135,1.952),(.094,-.068,1.855)],.024,.015),
        ([(.029,-.047,1.906),(.067,-.082,1.963),(.107,-.073,1.917),(.105,-.039,1.854)],.023,.013),
        ([(-.080,-.067,1.865),(-.121,-.10,1.932),(-.098,-.17,1.919),(-.060,-.169,1.835)],.025,.013),
        ([(-.062,-.095,1.876),(-.089,-.144,1.924),(-.072,-.176,1.887),(-.036,-.169,1.826)],.021,.011),
        ([(.005,-.136,1.89),(.041,-.17,1.934),(.067,-.183,1.889),(.077,-.146,1.819)],.018,.012),
        ([(-.090,-.043,1.827),(-.111,-.076,1.833),(-.113,-.09,1.797),(-.096,-.102,1.757)],.015,.010),
        ([(.090,-.045,1.830),(.115,-.060,1.843),(.119,-.08,1.795),(.103,-.095,1.766)],.016,.011),
    ]
    for index,(controls,width,depth) in enumerate(forelocks):
        controls=[(x,y,1.90+(z-1.90)*.52 if z>1.90 else z) for x,y,z in controls]
        sculpted_lock(f"Hair • signature forelock {index+1:02}",controls,width*1.10,depth*.70,coll,palette[index%3])
    hairline=[
        [(-.052,-.112,1.899),(-.025,-.164,1.902),(.014,-.174,1.864),(.043,-.157,1.849)],
        [(-.063,-.103,1.883),(-.048,-.165,1.873),(-.003,-.178,1.848),(.025,-.161,1.842)],
        [(-.038,-.118,1.909),(.002,-.158,1.909),(.045,-.172,1.872),(.065,-.148,1.840)],
        [(-.088,-.039,1.874),(-.111,-.070,1.903),(-.098,-.150,1.875),(-.070,-.16,1.810)],
    ]
    for index,controls in enumerate(hairline):
        sculpted_lock(f"Hair • fine swept hairline {index+1:02}",controls,.017,.005,coll,palette[index%3])
    # Individual eyebrow fibres follow the facial surface instead of floating cards.
    depsgraph=bpy.context.evaluated_depsgraph_get()
    evaluated=body.evaluated_get(depsgraph)
    for sign in (-1,1):
        verts,faces=[],[]
        for index in range(41):
            t=index/40
            x=sign*(.013+.056*t)
            z=1.797+.0045*math.sin(t*math.pi)-.003*t
            width=.0055*math.sin(math.pi*t)**.38*(1-t)**.20+.00008
            for row in range(7):
                hit,position,normal,_=evaluated.ray_cast(Vector((x,-1,z+width*(-1+row/3))),Vector((0,1,0)))
                if not hit:
                    raise ValueError("Eyebrow could not be fitted to face")
                verts.append(tuple(position+normal*(.0007+.0004*math.sin(math.pi*row/6))))
                if index and row:
                    a=(index-1)*7+row-1;faces.append((a,a+7,a+8,a+1))
        brow=mesh_object(f"Brow • fitted {'L' if sign>0 else 'R'}",verts,faces,coll,palette[0])
        subdivide(brow,1)
        for index in range(37):
            start=Vector(verts[index*7+1]);end=Vector(verts[(index+3)*7+5])
            curve_object(f"Brow fibre {sign}.{index}",[start,(start+end)/2+Vector((0,-.0005,0)),end],.00016,coll,palette[1])


def fabric_material():
    mat,shader=principled("Linen • unbleached woven flax",(.50,.385,.235),.84)
    shader.inputs["Sheen Weight"].default_value=.24
    shader.inputs["Sheen Roughness"].default_value=.8
    nodes,links=mat.node_tree.nodes,mat.node_tree.links
    coords=nodes.new("ShaderNodeTexCoord")
    wave_outputs=[]
    for direction in ("X","Y"):
        wave=nodes.new("ShaderNodeTexWave")
        wave.wave_type="BANDS"
        wave.bands_direction=direction
        wave.inputs["Scale"].default_value=135
        wave.inputs["Distortion"].default_value=.7
        wave.inputs["Detail Scale"].default_value=4
        links.new(coords.outputs["UV"],wave.inputs["Vector"])
        wave_outputs.append(wave.outputs["Color"])
    multiply=nodes.new("ShaderNodeMath")
    multiply.operation="MULTIPLY"
    for index,output in enumerate(wave_outputs):links.new(output,multiply.inputs[index])
    bump=nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value=.28
    bump.inputs["Distance"].default_value=.0006
    links.new(multiply.outputs[0],bump.inputs["Height"])
    links.new(bump.outputs["Normal"],shader.inputs["Normal"])
    noise=nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value=18
    noise.inputs["Detail"].default_value=3
    links.new(coords.outputs["UV"],noise.inputs["Vector"])
    ramp=nodes.new("ShaderNodeValToRGB")
    ramp.color_ramp.elements[0].color=(.33,.246,.138,1)
    ramp.color_ramp.elements[1].color=(.59,.47,.30,1)
    links.new(noise.outputs["Fac"],ramp.inputs["Fac"])
    links.new(ramp.outputs["Color"],shader.inputs["Base Color"])
    return mat


def create_linen(coll,body):
    linen=fabric_material()
    edge,_=principled("Linen • hem thread",(.32,.245,.145),.86)
    evaluated=body.evaluated_get(bpy.context.evaluated_depsgraph_get())
    def waist_point(theta,v):
        height=1.12-.058*v+.005*math.cos(theta*2)+.003*math.sin(theta*4)
        direction=Vector((math.sin(theta),-math.cos(theta),0))
        hit,position,normal,_=evaluated.ray_cast(Vector((0,0,height)),direction)
        if not hit:
            raise ValueError("Waistband ray missed body")
        radial=.0028*math.sin(theta*19+v*1.3)*math.sin(v*math.pi)
        return position+direction*(.009+radial)

    def panel_point(u,v,back=False):
        width=.151*(1-.47*v)
        x=u*width+.006*math.sin(v*4)*(1-u*u)
        hem=.047*(1-u*u)+.008*math.sin(u*6)
        z=1.107-v*(.272+hem)+(.016*v if back else 0)
        ray_origin=Vector((u*.151,1 if back else -1,1.105))
        direction=Vector((0,-1 if back else 1,0))
        hit,root_position,_,_=evaluated.ray_cast(ray_origin,direction)
        if not hit:
            raise ValueError("Linen attachment missed waist")
        outward=1 if back else -1
        fold=.010*math.sin(u*4*math.pi+.7*v)*(.15+.85*v)+.004*math.sin(u*7*math.pi-1.5*v)*v
        y=root_position.y+outward*(.007+.019*v+fold)
        hit,contact,_,_=evaluated.ray_cast(Vector((x,outward,z)),direction)
        if hit:
            clearance=contact.y+outward*.007
            y=max(y,clearance) if back else min(y,clearance)
        return (x,y,z)
    for back in (False,True):
        vertices,faces,uv=[],[],[]
        columns,rows=64,72
        for row in range(rows+1):
            for col in range(columns+1):
                u,v=-1+2*col/columns,row/rows
                vertices.append(panel_point(u,v,back))
                if row and col:
                    a=(row-1)*(columns+1)+col-1
                    faces.append((a,a+1,a+columns+2,a+columns+1))
                    uv.append((((col-1)/columns,(row-1)/rows),(col/columns,(row-1)/rows),(col/columns,v),((col-1)/columns,v)))
        name="Rear" if back else "Front"
        obj=mesh_object(f"Linen • {name.lower()} draped panel",vertices,faces,coll,linen,uv)
        subdivide(obj,1)
        thickness=obj.modifiers.new("Tailored fabric thickness","SOLIDIFY")
        thickness.thickness=.0022
        for side in (-1,1):
            points=[panel_point(side*.98,index/100,back) for index in range(101)]
            curve_object(f"{name} rolled side hem {side}",points,.00135,coll,linen)
        points=[panel_point(-1+2*index/100,.997,back) for index in range(101)]
        curve_object(f"{name} rolled bottom hem",points,.0014,coll,linen)
        for index in range(44):
            u=-.97+1.94*index/44
            points=[Vector(panel_point(u,.97,back)),Vector(panel_point(u+.019,.97,back))]
            for point in points:point.y+=.0018 if back else -.0018
            curve_object(f"{name} hem stitch {index:02}",points,.00036,coll,edge)
    # The waistband follows the waist, with gathered fabric and a folded upper edge.
    vertices,faces,uv=[],[],[]
    segments,rows=192,14
    for row in range(rows+1):
        v=row/rows
        for col in range(segments):
            theta=TAU*col/segments
            vertices.append(tuple(waist_point(theta,v)))
            if row:
                a=(row-1)*segments+col;b=(row-1)*segments+(col+1)%segments
                faces.append((a,b,b+segments,a+segments))
                uv.append(((col/segments,(row-1)/rows),((col+1)/segments,(row-1)/rows),((col+1)/segments,v),(col/segments,v)))
    band=mesh_object("Linen • gathered waistband",vertices,faces,coll,linen,uv)
    subdivide(band,1)
    solid=band.modifiers.new("Waistband fabric thickness","SOLIDIFY")
    solid.thickness=.003
    for height in (0,.95):
        points=[waist_point(t,height) for t in np.linspace(0,TAU,193)]
        curve_object("Waistband • folded edge",points,.0018,coll,linen,True)
    for offset in (-.006,.006):
        controls=[(.128,-.119,1.083),(.15,-.19,1.09),(.125,-.186,1.045),(.084,-.178,1.033)]
        points=[bezier(controls,t)+Vector((offset,0,0)) for t in np.linspace(0,1,45)]
        curve_object("Linen • side tie",points,.005,coll,linen)


def camera(name, position, target, scale, coll):
    camera_data = bpy.data.cameras.new(name)
    obj = bpy.data.objects.new(name, camera_data)
    coll.objects.link(obj)
    obj.location = position
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()
    camera_data.type = "ORTHO"
    camera_data.ortho_scale = scale
    camera_data.lens = 70
    return obj


def create_nails(body,joints,coll):
    material,shader=principled("Nails • natural keratin",(.38,.22,.145),.48)
    shader.inputs["Coat Weight"].default_value=.06
    evaluated=body.evaluated_get(bpy.context.evaluated_depsgraph_get())
    for side in ("l","r"):
        wrist=Vector(joints[f"{side}-hand"])
        hand_axis=(Vector(joints[f"{side}-finger-3-1"])-wrist).normalized()
        across=(Vector(joints[f"{side}-finger-2-1"])-Vector(joints[f"{side}-finger-5-1"])).normalized()
        hand_normal=across.cross(hand_axis).normalized()
        if hand_normal.z<0:hand_normal.negate()
        for finger in range(1,6):
            start=Vector(joints[f"{side}-finger-{finger}-3"])
            end=Vector(joints[f"{side}-finger-{finger}-4"])
            axis=(end-start).normalized()
            width_axis=axis.cross(hand_normal).normalized()
            normal=width_axis.cross(axis).normalized()
            center=start.lerp(end,.55)
            length=(end-start).length*.52
            width=.0062 if finger==1 else (.0046 if finger==5 else .0054)
            vertices,faces,uv=[],[],[]
            rows,columns=20,16
            for row in range(rows+1):
                v=-1+2*row/rows
                half_width=width*max(.05,1-v**8)**.5
                for column in range(columns+1):
                    u=-1+2*column/columns
                    target=center+axis*(v*length/2)+width_axis*(u*half_width)
                    hit,position,surface_normal,_=evaluated.ray_cast(target+normal*.026,-normal,distance=.044)
                    if not hit or (position-target).length>.022:
                        hit,position,surface_normal,_=evaluated.closest_point_on_mesh(target+normal*.005,distance=.022)
                    if not hit or (position-target).length>.023:
                        raise ValueError(f"Nail missed fingertip: {side}/{finger}")
                    vertices.append(tuple(position+surface_normal*.00045))
                    if row and column:
                        first=(row-1)*(columns+1)+column-1
                        faces.append((first,first+1,first+columns+2,first+columns+1))
                        uv.append((((column-1)/columns,(row-1)/rows),(column/columns,(row-1)/rows),(column/columns,row/rows),((column-1)/columns,row/rows)))
            nail=mesh_object(f"Nail • {side.upper()} {finger}",vertices,faces,coll,material,uv)
            subdivide(nail,1)
            thickness=nail.modifiers.new("Nail plate thickness","SOLIDIFY")
            thickness.thickness=.0003


def studio(coll):
    scene = bpy.context.scene
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 48
    scene.cycles.use_adaptive_sampling=True
    scene.cycles.adaptive_threshold=.025
    scene.cycles.adaptive_min_samples=12
    scene.cycles.max_bounces=8
    scene.cycles.transmission_bounces=4
    scene.cycles.use_denoising = True
    scene.cycles.device = "CPU"
    scene.world.color = (.2,.2,.2)
    scene.world.use_nodes = True
    scene.world.node_tree.nodes.get("Background").inputs["Color"].default_value = (.33,.39,.47,1)
    scene.world.node_tree.nodes.get("Background").inputs["Strength"].default_value = .35
    for name, position, energy, size, color in [
        ("Key • large warm softbox",(-3,-4,5),470,2.2,(1,.87,.73)),
        ("Fill • cool softbox",(3,-1,3),155,2.5,(.72,.83,1)),
        ("Rim • hair separation",(1,3,4),620,2.0,(1,.85,.62))]:
        lamp = bpy.data.lights.new(name,"AREA")
        lamp.energy, lamp.shape, lamp.size, lamp.color = energy,"DISK",size,color
        obj = bpy.data.objects.new(name,lamp)
        coll.objects.link(obj)
        obj.location = position
        obj.rotation_euler = (Vector((0,0,1)) - obj.location).to_track_quat("-Z","Y").to_euler()
    floor, _ = principled("Studio • warm neutral",(.105,.118,.128),.86)
    mesh_object("Studio ground",[(-200,-200,-.006),(200,-200,-.006),(200,200,-.006),(-200,200,-.006)],[(0,1,2,3)],coll,floor)
    scene.view_settings.view_transform = "AgX"
    scene.view_settings.look = "AgX - Medium High Contrast"
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.film_transparent = False
    scene.render.resolution_percentage = 100
    return {
        "front":camera("Front",(0,-5,1.00),(0,0,1.00),2.12,coll),
        "hero":camera("Three quarter",(3,-5,2.6),(0,0,1.02),2.20,coll),
        "back":camera("Back",(0,5,1.00),(0,0,1.00),2.12,coll),
        "side":camera("Side",(5,0,1.00),(0,0,1.00),2.12,coll),
        "portrait":camera("Face closeup",(1.3,-4,1.85),(0,-.015,1.735),.49,coll),
        "hand":camera("Hand closeup",(2.5,-1.6,2.5),(.585,-.31,1.11),.30,coll),
        "feet":camera("Feet closeup",(1.5,-3,1.3),(0,-.10,.09),.70,coll),
        "back_threequarter":camera("Back three quarter",(-3,5,2.4),(0,0,1.02),2.20,coll),
        "clay":camera("Clay form review",(3,-5,2.6),(0,0,1.02),2.20,coll),
    }


def validate_scene():
    reports={}
    for obj in bpy.data.objects:
        if obj.type!="MESH" or obj.hide_render:
            continue
        mesh=obj.data
        coordinates=np.empty(len(mesh.vertices)*3)
        mesh.vertices.foreach_get("co",coordinates)
        if not np.isfinite(coordinates).all():
            raise ValueError(f"Nonfinite coordinates in {obj.name}")
        editable=bmesh.new()
        editable.from_mesh(mesh)
        degenerate=sum(face.calc_area()<1e-14 for face in editable.faces)
        nonmanifold=sum(len(edge.link_faces)>2 for edge in editable.edges)
        loose=sum(not vertex.link_faces for vertex in editable.verts)
        if degenerate or nonmanifold or loose:
            raise ValueError(f"Invalid topology in {obj.name}: degenerate={degenerate}, nonmanifold={nonmanifold}, loose={loose}")
        reports[obj.name]={"vertices":len(mesh.vertices),"faces":len(mesh.polygons),"boundary_edges":sum(edge.is_boundary for edge in editable.edges),"quad_faces":sum(len(face.verts)==4 for face in editable.faces),"uv_layers":len(mesh.uv_layers)}
        if obj.name.startswith("Body"):
            unseen=set(editable.verts)
            components=0
            while unseen:
                pending=[unseen.pop()]
                components+=1
                while pending:
                    vertex=pending.pop()
                    for edge in vertex.link_edges:
                        neighbor=edge.other_vert(vertex)
                        if neighbor in unseen:
                            unseen.remove(neighbor)
                            pending.append(neighbor)
            if components!=1:
                raise ValueError(f"Body is not one connected surface: {components}")
            reports[obj.name]["connected_components"]=components
            if not mesh.shape_keys or "Concept • muscular surface sculpt" not in mesh.shape_keys.key_blocks:
                raise ValueError("Missing editable anatomy sculpt")
        if obj.name.startswith("Nail"):
            bounds=coordinates.reshape((-1,3))
            if np.linalg.norm(bounds.max(axis=0)-bounds.min(axis=0))>.045:
                raise ValueError(f"Nail extends outside fingertip: {obj.name}")
        editable.free()
    external_images=[image.name for image in bpy.data.images if image.source=="FILE" and not image.packed_file]
    if external_images:
        raise ValueError(f"Blend has unpacked image dependencies: {external_images}")
    result={"blender":bpy.app.version_string,"stage":"Static model, materials and review renders; unrigged","checks":{"finite_geometry":True,"no_degenerate_faces":True,"no_edges_with_more_than_two_faces":True,"no_loose_vertices":True,"single_connected_body":True,"textures_packed":True,"nails_fit_fingertips":True},"objects":reports}
    (OUT/"validation.json").write_text(json.dumps(result,indent=2)+"\n")
    print("V3 geometry and packed-dependency validation passed",flush=True)
    return result


def embed_reference():
    coll=make_collection("81 • CONCEPT REFERENCE")
    image=bpy.data.images.load(str(OUT/"reference/male_commoner_concept.png"))
    image.pack()
    obj=bpy.data.objects.new("Approved visual reference • original turnaround",None)
    coll.objects.link(obj)
    obj.empty_display_type="IMAGE"
    obj.data=image
    obj.empty_display_size=4
    obj.location=(0,1,1)
    obj.rotation_euler=(math.pi/2,0,0)
    obj.hide_render=True
    obj.hide_set(True)
    readme=OUT/"README.md"
    if readme.exists():
        block=bpy.data.texts.new("ABOUT THIS CHARACTER")
        block.write(readme.read_text())


def main():
    args_parser = argparse.ArgumentParser()
    args_parser.add_argument("--preview",action="store_true")
    args_parser.add_argument("--views",default="front,hero,back,side,back_threequarter,portrait,hand,feet,clay")
    args_parser.add_argument("--verify-saved",action="store_true")
    args = args_parser.parse_args(sys.argv[sys.argv.index("--")+1:] if "--" in sys.argv else [])
    verify_sources()
    if args.verify_saved:
        bpy.ops.wm.open_mainfile(filepath=str(OUT/"male_commoner_v3.blend"))
        validate_scene()
        return
    bpy.context.preferences.filepaths.save_version=0
    bpy.ops.object.select_all(action="SELECT")
    bpy.ops.object.delete(use_global=False)
    for coll in list(bpy.data.collections):
        if not coll.objects:
            bpy.data.collections.remove(coll)
    body_coll = make_collection("01 • BODY")
    eyes_coll = make_collection("02 • EYES")
    hair_coll = make_collection("03 • HAIR")
    cloth_coll = make_collection("04 • LINEN")
    detail_coll = make_collection("05 • NAILS")
    source_coll = make_collection("80 • EDITABLE CONTROL CAGE")
    studio_coll = make_collection("90 • STUDIO")
    raw, converted, uv, faces, groups, joints, convert = anatomical_source()
    proxy_path=SYSTEM/"proxymeshes/male_muscle_13290/male_muscle_13290"
    _,proxy_uv,proxy_faces,proxy_groups=read_obj(proxy_path.with_suffix(".obj"))
    proxy_coords=convert(fit_proxy(proxy_path.with_suffix(".proxy"),raw))
    proxy_faces=[("body",corners) for _,corners in proxy_faces]
    proxy_groups={"body":set(corner[0] for _,corners in proxy_faces for corner in corners)}
    body = create_body(proxy_coords,proxy_uv,proxy_faces,proxy_groups,body_coll,skin_material())
    sculpt_anatomy(body,joints,source_coll)
    create_eyes(raw,convert,eyes_coll)
    create_hair(hair_coll,body)
    create_linen(cloth_coll,body)
    create_nails(body,joints,detail_coll)
    embed_reference()
    cameras = studio(studio_coll)
    PREVIEWS.mkdir(parents=True,exist_ok=True)
    (OUT/"landmarks.json").write_text(json.dumps({name:value.tolist() for name,value in joints.items()},indent=2)+"\n")
    scene = bpy.context.scene
    scene.unit_settings.system="METRIC"
    scene.unit_settings.length_unit="METERS"
    scene["asset_name"]="Male commoner v3"
    scene["asset_stage"]="Static source model; art review candidate; no rig or animations"
    scene["source_license"]="MakeHuman anatomical assets: CC0. See embedded ABOUT THIS CHARACTER."
    scene.camera = cameras["hero"]
    scene.render.resolution_x = 900 if args.preview else 1800
    scene.render.resolution_y = 1100 if args.preview else 2200
    scene.cycles.samples = 24 if args.preview else 96
    bpy.ops.object.select_all(action="DESELECT")
    body.select_set(True)
    bpy.context.view_layer.objects.active = body
    for screen in bpy.data.screens:
        for area in screen.areas:
            if area.type=="VIEW_3D":
                area.spaces.active.region_3d.view_perspective="CAMERA"
                area.spaces.active.shading.type="MATERIAL"
    validate_scene()
    bpy.ops.wm.save_as_mainfile(filepath=str(OUT/"male_commoner_v3.blend"),compress=True)
    clay,_=principled("Review • neutral clay",(.21,.235,.26),.66)
    for view in args.views.split(","):
        scene.camera = cameras[view]
        bpy.context.view_layer.material_override=clay if view=="clay" else None
        scene.render.filepath = str(PREVIEWS/("male_commoner_v3_"+view+".png"))
        bpy.ops.render.render(write_still=True)
    bpy.context.view_layer.material_override=None
    scene.camera=cameras["hero"]
    bpy.ops.wm.save_as_mainfile(filepath=str(OUT/"male_commoner_v3.blend"),compress=True)
    print("V3 renders complete",flush=True)


if __name__ == "__main__":
    main()
