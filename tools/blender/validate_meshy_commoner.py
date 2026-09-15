import bpy,math,json,numpy as np
from pathlib import Path
rig=bpy.data.objects['commoner_rig'];mesh=bpy.data.objects['meshy_commoner'];scene=bpy.context.scene
report={}
for name in ['meshy_commoner','meshy_commoner_low']:
 obj=bpy.data.objects[name]
 for vertex in obj.data.vertices:
  if len(vertex.groups)>4 or abs(sum(group.weight for group in vertex.groups)-1)>.002:raise RuntimeError('Invalid weights '+name)
  if abs(vertex.co.x)>.30 and .87<vertex.co.z<1.20:
   if any(obj.vertex_groups[group.group].name in ['pelvis','spine','chest'] and group.weight>.001 for group in vertex.groups):raise RuntimeError('Hand incorrectly weighted to torso')
for clip in ['idle','walk','carry_idle','carry_walk']:
 rig.animation_data.action=bpy.data.actions['Meshy '+clip]
 first=None;last=None;minimum=100;maximum=-100;widths=[];wrist_rotations=[]
 for frame in range(1,50):
  scene.frame_set(frame);bpy.context.view_layer.update()
  for bone in rig.pose.bones:
   if max(abs(value-1) for value in bone.matrix.to_scale())>.002:raise RuntimeError('Non-unit scale '+bone.name)
  if clip.startswith('carry'):
   for side in ['l','r']:
    forearm=rig.pose.bones['forearm.'+side];hand=rig.pose.bones['hand.'+side]
    forearm_deform=forearm.matrix.to_quaternion() @ forearm.bone.matrix_local.to_quaternion().inverted()
    hand_deform=hand.matrix.to_quaternion() @ hand.bone.matrix_local.to_quaternion().inverted()
    wrist_rotation=forearm_deform.rotation_difference(hand_deform).angle
    wrist_rotation=min(wrist_rotation,2*math.pi-wrist_rotation)
    wrist_rotations.append(math.degrees(wrist_rotation))
    # Independent overhead swings previously twisted this joint almost shut.
    if wrist_rotation>math.radians(30):raise RuntimeError('Excessive wrist deformation '+clip+' '+side)
    if forearm.vector.angle(hand.vector)>math.radians(1):raise RuntimeError('Carry wrist is not aligned with forearm')
  evaluated=mesh.evaluated_get(bpy.context.evaluated_depsgraph_get())
  positions=np.empty(len(evaluated.data.vertices)*3,dtype=np.float64);evaluated.data.vertices.foreach_get('co',positions);positions=positions.reshape(-1,3)
  if not np.isfinite(positions).all():raise RuntimeError('Nonfinite geometry')
  minimum=min(minimum,float(positions[:,2].min()));maximum=max(maximum,float(positions[:,2].max()))
  if frame==1:first=positions.copy()
  if frame==49:last=positions.copy()
  widths.append(abs(rig.pose.bones['foot.l'].head.x-rig.pose.bones['foot.r'].head.x))
 seam=float(np.linalg.norm(first-last,axis=1).max())
 if seam>.0001:raise RuntimeError('Animation loop seam '+clip+': '+str(seam))
 if max(abs(width-.21) for width in widths)>.001:raise RuntimeError('Incorrect walk width')
 report[clip]={'loop_seam_m':seam,'min_height_m':minimum,'max_height_m':maximum,'foot_width_m':[min(widths),max(widths)]}
 if wrist_rotations:report[clip]['max_wrist_deformation_degrees']=max(wrist_rotations)
rig.animation_data.action=bpy.data.actions['Meshy idle'];scene.frame_set(1)
bpy.context.view_layer.objects.active=mesh
for obj in scene.objects:obj.select_set(obj==mesh)
bpy.ops.wm.save_as_mainfile(filepath='/Users/park/projects/origin_go/art_source/characters/male_commoner_v4/commoner_rigged.blend',compress=True)
Path('/Users/park/projects/origin_go/art_source/characters/male_commoner_v4/validation.json').write_text(json.dumps(report,indent=2)+'\n')
result=report
