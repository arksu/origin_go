"""Run with Blender Python or MCP: run(config_path, output_directory).

Creates an isolated scene; never saves or changes the source scene.
Penetration counts are nearest-surface candidates, not a watertight collision proof.
"""
import json
import html
import math
from pathlib import Path

import bpy
from mathutils import Vector
from mathutils.bvhtree import BVHTree


def surface(obj):
    evaluated = obj.evaluated_get(bpy.context.evaluated_depsgraph_get())
    mesh = evaluated.to_mesh()
    try:
        vertices = [evaluated.matrix_world @ vertex.co for vertex in mesh.vertices]
        faces = [tuple(face.vertices) for face in mesh.polygons]
        return vertices, BVHTree.FromPolygons(vertices, faces)
    finally:
        evaluated.to_mesh_clear()


def run(config_path, output_directory, strict=False):
    config = json.loads(Path(config_path).read_text())
    output = Path(output_directory)
    output.mkdir(parents=True, exist_ok=False)
    original_scene = bpy.context.window.scene
    scene = bpy.data.scenes.new('Equipment pose review')
    created = []
    blocks = []
    report = {'config': config, 'source': bpy.data.filepath, 'poses': [],
              'metric_note': 'Nearest outward body surface: negative signed clearance is a penetration candidate. Open body meshes and seams require image review.'}
    try:
        bpy.context.window.scene = scene
        source_shirt = next(o for o in bpy.data.objects if o.name == config['garment'] and o.library is None)
        source_body = next(o for o in bpy.data.objects if o.name == config['body'])
        source_rig = next(m.object for m in source_shirt.modifiers if m.type == 'ARMATURE')
        groups = {group.index: group.name for group in source_shirt.vertex_groups}
        invalid_vertices = []
        for vertex in source_shirt.data.vertices:
            weights = [g.weight for g in vertex.groups if g.weight > 0]
            if (not weights or len(weights) > config['max_influences'] or
                    any(not math.isfinite(g.weight) or g.weight < 0 for g in vertex.groups) or abs(sum(weights)-1) > 1e-4 or
                    any(groups[g.group] not in source_rig.data.bones for g in vertex.groups if g.weight > 0)):
                invalid_vertices.append(vertex.index)
        report['skin_contract'] = {'invalid_vertices': invalid_vertices, 'max_influences': config['max_influences']}
        if invalid_vertices:
            raise ValueError(f'Invalid skin weights on {len(invalid_vertices)} vertices')
        rig = source_rig.copy()
        rig.data = source_rig.data.copy()
        blocks.append(rig.data)
        rig.animation_data_clear()
        scene.collection.objects.link(rig)
        created.append(rig)
        meshes = []
        for source, color in [(source_body, (0.95, 0.23, 0.18, 1)), (source_shirt, (0.25, 0.5, 0.65, 1))]:
            obj = source.copy()
            obj.data = source.data.copy()
            blocks.append(obj.data)
            obj.animation_data_clear()
            obj.parent = None
            obj.matrix_world = source.matrix_world.copy()
            for modifier in obj.modifiers:
                if modifier.type == 'ARMATURE':
                    modifier.object = rig
            obj.hide_viewport = False
            obj.hide_render = False
            obj.color = color
            scene.collection.objects.link(obj)
            created.append(obj)
            meshes.append(obj)
        body, garment = meshes
        camera_data = bpy.data.cameras.new('Pose review camera')
        blocks.append(camera_data)
        camera = bpy.data.objects.new('Pose review camera', camera_data)
        created.append(camera)
        scene.collection.objects.link(camera)
        scene.camera = camera
        scene.render.engine = 'BLENDER_WORKBENCH'
        scene.display.shading.light = 'STUDIO'
        scene.display.shading.color_type = 'OBJECT'
        scene.display.shading.show_shadows = False
        scene.display.shading.show_cavity = True
        scene.display.shading.background_type = 'WORLD'
        scene.world = bpy.data.worlds.new('Pose review background')
        blocks.append(scene.world)
        scene.world.color = (0.025, 0.025, 0.025)
        scene.render.resolution_x = 640
        scene.render.resolution_y = 640
        scene.render.resolution_percentage = 100
        scene.render.image_settings.file_format = 'PNG'
        camera_data.type = 'ORTHO'
        camera_data.ortho_scale = 2.3
        for case in config['poses']:
            rig.animation_data_clear()
            for bone in rig.pose.bones:
                bone.matrix_basis.identity()
            rig.data.pose_position = 'REST' if case['action'] is None else 'POSE'
            if case['action']:
                action = next(a for a in bpy.data.actions if a.name == case['action'] and a.library is None)
                rig.animation_data_create().action = action
                if action.slots:
                    rig.animation_data.action_slot = action.slots[0]
            scene.frame_set(case['frame'])
            bpy.context.view_layer.update()
            positions, _ = surface(garment)
            _, body_tree = surface(body)
            penetrations = []
            # Vertex-only tests miss a large triangle cutting through a shoulder.
            evaluated = garment.evaluated_get(bpy.context.evaluated_depsgraph_get())
            evaluated_mesh = evaluated.to_mesh()
            try:
                evaluated_mesh.calc_loop_triangles()
                centers = [sum((positions[i] for i in triangle.vertices), Vector()) / 3
                           for triangle in evaluated_mesh.loop_triangles]
            finally:
                evaluated.to_mesh_clear()
            for index, point in enumerate(positions + centers):
                nearest, normal, face, distance = body_tree.find_nearest(point)
                if nearest is not None and distance < config['proximity_limit']:
                    signed = (point - nearest).dot(normal)
                    if signed < -config['penetration_tolerance']:
                        penetrations.append((index, -signed))
            result = {'name': case['name'], 'action': case['action'], 'frame': case['frame'],
                      'penetration_candidates': len(penetrations),
                      'max_penetration': max((depth for _, depth in penetrations), default=0),
                      'vertex_candidates': sum(index < len(positions) for index, _ in penetrations),
                      'triangle_candidates': sum(index >= len(positions) for index, _ in penetrations),
                      'status': 'needs_review' if penetrations else 'no_sampled_penetration',
                      'images': []}
            target = Vector((0, 0, 1.35))
            for view, offset in [('front', (0, -4, 0)), ('back', (0, 4, 0)), ('game', (3, -4, 3))]:
                camera.location = target + Vector(offset)
                camera.rotation_euler = (target-camera.location).to_track_quat('-Z', 'Y').to_euler()
                filename = f"{case['name']}-{view}.png"
                scene.render.filepath = str(output / filename)
                bpy.ops.render.render(write_still=True)
                result['images'].append(filename)
            report['poses'].append(result)
        report['status'] = 'needs_review' if any(p['penetration_candidates'] for p in report['poses']) else 'visual_review_required'
        (output / 'report.json').write_text(json.dumps(report, indent=2))
        sections = []
        for pose in report['poses']:
            images = ''.join(f'<img width="320" src="{html.escape(name)}" alt="{html.escape(name)}">' for name in pose['images'])
            sections.append(f'<h2>{html.escape(pose["name"])}</h2><p>{pose["status"]}: {pose["vertex_candidates"]} vertices, {pose["triangle_candidates"]} triangle centers; max depth {pose["max_penetration"]:.5f}</p>{images}')
        (output / 'index.html').write_text('<!doctype html><meta charset="utf-8"><title>Equipment pose review</title><style>body{background:#222;color:#eee;font-family:sans-serif}img{max-width:32%}</style><h1>Equipment pose review</h1><p>Body: red. Equipment: blue. Openings at neck, cuffs and hem need visual assessment. This report does not certify all animation frames.</p>' + ''.join(sections))
        print(json.dumps(report, indent=2))
    finally:
        bpy.context.window.scene = original_scene
        for obj in reversed(created):
            bpy.data.objects.remove(obj, do_unlink=True)
        bpy.data.scenes.remove(scene)
        for block in blocks:
            if block.users == 0:
                if isinstance(block, bpy.types.Mesh): bpy.data.meshes.remove(block)
                elif isinstance(block, bpy.types.Armature): bpy.data.armatures.remove(block)
                elif isinstance(block, bpy.types.Camera): bpy.data.cameras.remove(block)
                elif isinstance(block, bpy.types.World): bpy.data.worlds.remove(block)
    if strict and report['status'] == 'needs_review':
        raise AssertionError(f'Equipment penetration candidates found; inspect {output / "index.html"}')
    return report
