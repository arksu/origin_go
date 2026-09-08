"""Inspect deformation quality and render enlarged arm diagnostics."""
import argparse
import json
import math
import sys
from pathlib import Path
import bpy
import numpy as np

sys.dont_write_bytecode = True
sys.path.insert(0,str(Path(__file__).resolve().parent))
import animate_male_commoner_walk as walk

parser=argparse.ArgumentParser()
parser.add_argument("--label",default="after",choices=("before","after"))
arguments=parser.parse_args(sys.argv[sys.argv.index("--")+1:] if "--" in sys.argv else [])
label=arguments.label

bpy.ops.wm.open_mainfile(filepath=str(walk.OUT/"male_commoner_walk.blend"))
scene=bpy.context.scene
body=next(obj for obj in walk.studio.character_objects() if obj.name.startswith("Body"))
rig=bpy.data.objects["Commoner • walk skeleton"]
actor=bpy.data.objects["Walk • MODEL DIRECTION"]
destination=walk.OUT/"arm_review"
destination.mkdir(exist_ok=True)
actor.rotation_euler.z=math.radians(270)

def coordinates():
    mesh=body.evaluated_get(bpy.context.evaluated_depsgraph_get()).data
    result=np.empty(len(mesh.vertices)*3)
    mesh.vertices.foreach_get("co",result)
    return result.reshape((-1,3))

deform=next(modifier for modifier in body.modifiers if modifier.type=="ARMATURE")
deform.show_viewport=False
bpy.context.view_layer.update()
rest=coordinates()
print("BODY_MATRIX",list(map(list,body.matrix_world)),"RIG_MATRIX",list(map(list,rig.matrix_world)),flush=True)
print("REST_BOUNDS",rest.min(axis=0).tolist(),rest.max(axis=0).tolist(),flush=True)
print("MODIFIERS",[(m.type,m.show_viewport,m.show_render) for m in body.modifiers],flush=True)
for vertex in body.data.vertices:
    group_names={body.vertex_groups[group.group].name:group.weight for group in vertex.groups}
    if group_names.get("head",0)>.5 and rest[vertex.index,0]>.32:
        print("SUSPICIOUS_HEAD",vertex.index,"rest",list(rest[vertex.index]),"base",list(vertex.co),"weights",group_names,flush=True)
        break
deform.show_viewport=True
edges=np.array([list(edge.vertices) for edge in body.data.edges])
lengths=np.linalg.norm(rest[edges[:,1]]-rest[edges[:,0]],axis=1)
arm=(np.abs(rest[:,0])>.30)&(rest[:,2]>.82)
arm_edges=arm[edges].all(axis=1)&(lengths>1e-6)
samples=[]
for frame in (1,7,13,19,25,31,37,43):
    scene.frame_set(frame)
    bpy.context.view_layer.update()
    posed=coordinates()
    ratios=np.linalg.norm(posed[edges[:,1]]-posed[edges[:,0]],axis=1)[arm_edges]/lengths[arm_edges]
    entry={"frame":frame,"edge_stretch_percentiles":np.percentile(ratios,[0,1,5,50,95,99,100]).tolist()}
    samples.append(entry)
    print("ARM_STRETCH",entry,flush=True)
    if frame in (1,25):
        walk.studio.render(destination/f"{label}_w_{frame:02}.png",(640,768),48)
for name in ("upper_arm.l","forearm.l","hand.l","upper_arm.r","forearm.r","hand.r"):
    bone=rig.pose.bones[name]
    print("BONE",name,"head",list(bone.head),"tail",list(bone.tail),"basis_euler",list(bone.matrix_basis.to_euler()),flush=True)
names=[group.name for group in body.vertex_groups]
binding_checks=[]
for side,sign in (("l",1),("r",-1)):
    shoulder=np.array(walk.JOINTS[f"{side}-shoulder"])
    elbow=np.array(walk.JOINTS[f"{side}-elbow"])
    wrist=np.array(walk.JOINTS[f"{side}-hand"])
    for segment,start,end in (("upper",shoulder,elbow),("forearm",elbow,wrist)):
        axis=end-start; along=(rest-start)@axis/np.dot(axis,axis)
        radial=np.linalg.norm(rest-start-along[:,None]*axis,axis=1)
        region=(along>.2)&(along<.8)&(radial<.14)&(rest[:,0]*sign>.32)
        influence={}
        for vertex_index in np.flatnonzero(region):
            for group in body.data.vertices[int(vertex_index)].groups:
                influence[names[group.group]]=influence.get(names[group.group],0)+group.weight
        print("WEIGHT_REGION",side,segment,int(region.sum()),sorted(influence.items(),key=lambda item:-item[1])[:8],flush=True)
        primary=f"upper_arm.{side}" if segment=="upper" else f"forearm.{side}"
        check={"side":side,"region":segment,"vertices":int(region.sum()),"primary_bone":primary,"primary_mean_weight":influence.get(primary,0)/region.sum(),"head_mean_weight":influence.get("head",0)/region.sum()}
        binding_checks.append(check)
        if label=="after":
            assert check["primary_mean_weight"]>.8,check
            assert check["head_mean_weight"]<.001,check
if label=="after":
    assert max(sample["edge_stretch_percentiles"][-2] for sample in samples)<1.6,"Excessive arm surface stretching"
    assert max(sample["edge_stretch_percentiles"][-1] for sample in samples)<5,"Arm surface spike"
(destination/f"{label}_binding.json").write_text(json.dumps(binding_checks,indent=2)+"\n")
(destination/f"{label}_deformation.json").write_text(json.dumps(samples,indent=2)+"\n")
print("ARM_INSPECTION_COMPLETE",flush=True)
