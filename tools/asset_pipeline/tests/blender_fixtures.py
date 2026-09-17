"""Disposable integration sources. Never accepts a production source as input."""
import argparse
import json
import math
import sys
from pathlib import Path

import bpy

parser = argparse.ArgumentParser()
parser.add_argument('--request', required=True)
args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:])
request = json.loads(Path(args.request).read_text())
source = Path(request['source'])
assert source.name == 'fixture.blend' and source.parent.name.startswith('asset-pipeline-test-')
options = request.get('overrides', {})

if request.get('edit'):
    bpy.ops.wm.open_mainfile(filepath=str(source), load_ui=False, use_scripts=False)
else:
    bpy.ops.object.select_all(action='SELECT')
    bpy.ops.object.delete(use_global=False)
    collection = bpy.data.collections.new('EXPORT')
    bpy.context.scene.collection.children.link(collection)
    mesh = bpy.data.meshes.new('TriangleGeometry')
    mesh.from_pydata([(0, 0, 0), (1, 0, 0), (0, 0, 1)], [], [(0, 1, 2)])
    model = bpy.data.objects.new('Body', mesh)
    collection.objects.link(model)
    model['lod'] = 0
    if options.get('runtimeExtras'):
        model['skinning'] = options.get('skinning', 'linear')
    if options.get('customExtras'):
        model['editor_source_path'] = str(source)
        model['last_preview_action'] = 'private-editor-state'
    if request['kind'] == 'character':
        rig = bpy.data.objects.new('Rig', bpy.data.armatures.new('Skeleton'))
        collection.objects.link(rig)
        bpy.context.view_layer.objects.active = rig
        rig.select_set(True)
        bpy.ops.object.mode_set(mode='EDIT')
        root = rig.data.edit_bones.new('root')
        root.head, root.tail = (0, 0, 0), (0, 0, 1)
        hand = rig.data.edit_bones.new('hand.r')
        hand.head, hand.tail, hand.parent = (0, 0, 1), (0, 0, 2), root
        bpy.ops.object.mode_set(mode='OBJECT')
        model.parent = rig
        modifier = model.modifiers.new('Skin', 'ARMATURE')
        modifier.object = rig
        group = model.vertex_groups.new(name='root')
        group.add([0, 1, 2], 1.0, 'REPLACE')
        if options.get('invalidWeights'):
            group.remove([2])
        if options.get('tooManyInfluences'):
            group.add([0, 1, 2], 0.5, 'REPLACE')
            model.vertex_groups.new(name='hand.r').add([0, 1, 2], 0.5, 'REPLACE')
        socket = bpy.data.objects.new('socket_hand_right', None)
        collection.objects.link(socket)
        socket.parent, socket.parent_type, socket.parent_bone = rig, 'BONE', 'hand.r'
        socket.location = (0.25, 0.5, -0.75)
        socket.rotation_euler = (0.2, 0.3, 0.4)
        if options.get('missingSocket'):
            bpy.data.objects.remove(socket, do_unlink=True)
        for action_name, amplitude in [('idle', 0.1), ('walk', 0.5)]:
            rig.animation_data_create()
            rig.animation_data.action = bpy.data.actions.new(action_name)
            rig.animation_data.action.use_fake_user = True
            for frame, angle in [(1, 0), (6, amplitude), (12, 0.7 if options.get('badLoop') else 0)]:
                rig.pose.bones['hand.r'].rotation_mode = 'XYZ'
                rig.pose.bones['hand.r'].rotation_euler.z = angle
                rig.pose.bones['hand.r'].keyframe_insert('rotation_euler', frame=frame)
            if options.get('mask'):
                rig.animation_data.action['asset_pipeline_channel_mask'] = options['mask']
            if options.get('singularBetweenKeys'):
                bone = rig.pose.bones['hand.r']
                bone.scale = (1, 1, 1)
                bone.keyframe_insert('scale', frame=1)
                bone.keyframe_insert('scale', frame=12)
                for layer in rig.animation_data.action.layers:
                    for strip in layer.strips:
                        for bag in strip.channelbags:
                            for curve in bag.fcurves:
                                if not curve.data_path.endswith('.scale'):
                                    continue
                                first, last = curve.keyframe_points
                                first.handle_right_type = last.handle_left_type = 'FREE'
                                first.handle_right = (1 + 11 / 3, -1 / 3)
                                last.handle_left = (1 + 22 / 3, -1 / 3)
        if options.get('driver'):
            rig.driver_add('location', 0).driver.expression = '__import__("os").getpid()'
        if options.get('constraint'):
            constraint = rig.pose.bones['hand.r'].constraints.new('COPY_ROTATION')
            constraint.target, constraint.subtarget = rig, 'root'
            constraint.influence = 0.5
        if options.get('savedNla'):
            track = rig.animation_data.nla_tracks.new()
            track.strips.new('Saved editor action', 1, bpy.data.actions['walk'])
        if options.get('unsafeConstraint'):
            rig.pose.bones['hand.r'].constraints.new('SPLINE_IK')
        if options.get('undeclaredPoleTarget'):
            target = bpy.data.objects.new('OutsideExportPole', None)
            bpy.context.scene.collection.objects.link(target)
            constraint = rig.pose.bones['hand.r'].constraints.new('IK')
            constraint.pole_target = target
    if request['kind'] == 'equipment':
        for name, translation, angle in [('grip_right', (1, 2, 3), math.pi / 2), ('grip_left', (-2, 1, 0.5), -math.pi / 2)]:
            grip = bpy.data.objects.new(name, None)
            collection.objects.link(grip)
            grip.location = translation
            grip.rotation_euler.z = angle
            if options.get('badGrip'):
                grip.scale.x = 2
    if options.get('duplicateNames'):
        for name in ['same.name', 'samename']:
            collection.objects.link(bpy.data.objects.new(name, None))
    if options.get('nonFinite'):
        mesh.vertices[0].co.x = float('nan')
    if options.get('modifier'):
        model.modifiers.new('Double sided thickness', 'SOLIDIFY').thickness = 0.1
    if options.get('packedTexture'):
        image = bpy.data.images.new('PackedColor', 2, 2)
        image.pixels = [0.25, 0.5, 0.75, 1] * 4
        image.pack()
        material = bpy.data.materials.new('Surface')
        material.use_nodes = True
        if options.get('runtimeExtras'):
            material['region'] = options.get('region', 'textured')
            material['editor_source_path'] = str(source)
        texture = material.node_tree.nodes.new('ShaderNodeTexImage')
        texture.image = image
        material.node_tree.links.new(texture.outputs['Color'], material.node_tree.nodes.get('Principled BSDF').inputs['Base Color'])
        model.data.materials.append(material)
        model.data.uv_layers.new(name='UVMap')
    if options.get('unbakedModifier'):
        model.modifiers.new('Undeclared simulation', 'CLOTH')
    if options.get('hidden'):
        model.hide_viewport = True
        collection.hide_viewport = True
    if options.get('externalImage'):
        image = bpy.data.images.new('External', 1, 1)
        image.use_fake_user = True
        image.filepath_raw = str(source.parent / 'undeclared.png')
        image.file_format = 'PNG'
        image.save()
        image.source = 'FILE'
    if options.get('externalLibrary'):
        library_file = source.parent / 'undeclared.blend'
        bpy.data.libraries.write(str(library_file), {model})
        with bpy.data.libraries.load(str(library_file), link=True) as (available, loaded):
            loaded.objects = [available.objects[0]]
        for obj in loaded.objects:
            bpy.context.scene.collection.objects.link(obj)
    preview = bpy.data.collections.new('PREVIEW')
    bpy.context.scene.collection.children.link(preview)
    preview.objects.link(bpy.data.objects.new('NeverExportPreview', None))

if options.get('actionEdit'):
    edit = options['actionEdit']
    rig = bpy.data.objects['Rig']
    rig.animation_data.action = bpy.data.actions[edit['action']]
    rig.animation_data.action_slot = rig.animation_data.action.slots[0]
    bone = rig.pose.bones[edit['bone']]
    bone.rotation_mode = 'XYZ'
    bone.rotation_euler.z = edit['rotationZ']
    bone.keyframe_insert('rotation_euler', frame=edit['frame'])
if options.get('restEdit'):
    rig = bpy.data.objects['Rig']
    bpy.context.view_layer.objects.active = rig
    bpy.ops.object.mode_set(mode='EDIT')
    rig.data.edit_bones['hand.r'].tail.x += 0.2
    bpy.ops.object.mode_set(mode='OBJECT')
if options.get('socketEdit'):
    bpy.data.objects['socket_hand_right'].location.x += 0.1
if options.get('textureEdit'):
    image = bpy.data.images['PackedColor']
    image.pixels = [0.75, 0.1, 0.25, 1] * 4
    image.pack()
if options.get('modifierEdit'):
    bpy.data.objects['Body'].modifiers.new('Thickness', 'SOLIDIFY').thickness = 0.1
if options.get('meshTransformEdit'):
    bpy.data.objects['Body'].location.x += 0.2
if options.get('removeAction'):
    bpy.data.actions.remove(bpy.data.actions[options['removeAction']])

if request['kind'] == 'character':
    rig = bpy.data.objects['Rig']
    rig.animation_data.action = bpy.data.actions[options.get('savedAction', 'walk')]
bpy.context.scene.frame_set(options.get('savedFrame', 12))
if options.get('savedPose') and request['kind'] == 'character':
    rig.pose.bones['root'].location = (8, 9, 10)
if options.get('savedSingularPose') and request['kind'] == 'character':
    rig.pose.bones['root'].scale = (0, 0, 0)
if options.get('missingExport'):
    bpy.data.collections['EXPORT'].name = 'MISSING_EXPORT'
bpy.ops.wm.save_as_mainfile(filepath=str(source))
