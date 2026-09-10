"""Deterministic pixel contours from animated geometry and fixed material ramps."""
from __future__ import annotations

import json
from pathlib import Path

import numpy as np
from PIL import Image, ImageColor

from bake_male_commoner_v3_pixel import CONFIG as BASE_CONFIG

OUT = Path(__file__).resolve().parents[2] / "art_source/characters/male_commoner_v3/pixel_style/walk"
FINISH = json.loads((OUT / "pixel_finish.json").read_text())
COLORS = list(dict.fromkeys([FINISH["outer_ink"], FINISH["inner_ink"]] + [color for ramp in FINISH["ramps"].values() for color in ramp]))
CONFIG = {**BASE_CONFIG, "palette": COLORS, "outline": FINISH["outer_ink"]}


def shifted(values, rows, columns):
    result = np.zeros_like(values)
    height, width = values.shape[:2]
    result[max(0,rows):min(height,height+rows), max(0,columns):min(width,width+columns)] = values[max(0,-rows):min(height,height-rows), max(0,-columns):min(width,width-columns)]
    return result


def neighbours(mask):
    return [shifted(mask, rows, columns) for rows in (-1, 0, 1) for columns in (-1, 0, 1) if rows or columns]


def load_geometry(path, width, height, geometry_path=None):
    if geometry_path is None:
        direction, count = path.parent.name, int(path.parent.parent.name)
        sample = int(path.stem) * 48 // count
        geometry_path = OUT / "geometry" / f"{direction}_{sample:02}.npz"
    with np.load(geometry_path) as buffers:
        def blocks(name):
            return buffers[name].reshape(height,2,width,2).transpose(0,2,1,3).reshape(height,width,4)
        materials = blocks("material")
        counts = np.stack([(materials == label).sum(axis=2) for label in range(1,6)], axis=2)
        material = np.argmax(counts,axis=2).astype(np.uint8)+1
        material[counts.max(axis=2)==0] = 0
        selected = (materials == material[:,:,None]) & (material[:,:,None]>0)
        depth_samples = np.where(selected,blocks("depth"),np.inf)
        ordered = np.sort(depth_samples,axis=2)
        middle = np.maximum(0,(selected.sum(axis=2)-1)//2)
        depth = np.take_along_axis(ordered,middle[:,:,None],axis=2)[:,:,0]
        depth[material==0]=0
        heights = (blocks("height")*selected).sum(axis=2)/np.maximum(1,selected.sum(axis=2))
    return material, depth, heights


def quantize_materials(rgb, material):
    result = np.zeros(rgb.shape,dtype=np.uint8)
    ramps = {0:COLORS,1:FINISH["ramps"]["skin"],2:FINISH["ramps"]["hair"],3:FINISH["ramps"]["linen"],4:FINISH["ramps"]["eyes"],5:FINISH["ramps"]["hair"]}
    adjusted = np.clip((rgb.astype(float)-127.5)*FINISH["contrast"]+127.5,0,255)
    for label, colors in ramps.items():
        mask = material==label
        palette = np.array([ImageColor.getrgb(color) for color in colors],dtype=float)
        distances = np.sum((adjusted[mask,None,:]-palette[None,:,:])**2 * [2,4,3],axis=2)
        result[mask] = palette[np.argmin(distances,axis=1)].astype(np.uint8)
    return result


def remove_isolated_colors(rgb, opaque, material, heights):
    """Merge speckles inside broad surfaces, retaining facial features and edges."""
    original = rgb.copy()
    surrounded = sum(neighbours(opaque)) >= 7
    eligible = opaque & surrounded & (((heights < 1.40) & np.isin(material,[1,3])) | (material==2))
    same = sum(np.all(shifted(original,rows,columns)==original,axis=2) for rows,columns in ((0,1),(0,-1),(1,0),(-1,0)))
    for row, column in np.argwhere(eligible & (same==0)):
        patch = original[row-1:row+2,column-1:column+2].reshape(-1,3)
        colors, counts = np.unique(patch,axis=0,return_counts=True)
        winner = int(np.argmax(counts))
        if counts[winner] >= 5:
            rgb[row,column] = colors[winner]
    return rgb


def internal_lines(material, depth, heights, opaque):
    lines = np.zeros(opaque.shape,dtype=bool)
    material_lines = np.zeros_like(lines)
    for rows, columns in ((0,1),(0,-1),(1,0),(-1,0)):
        other_material = shifted(material,rows,columns)
        other_depth = shifted(depth,rows,columns)
        valid = opaque & shifted(opaque,rows,columns) & (material>0) & (other_material>0)
        # Draw only on the nearer surface, so a crossing arm gets one contour.
        threshold = np.where(material==2,FINISH["hair_depth_edge_m"],FINISH["depth_edge_m"])
        lines |= valid & (other_depth-depth>threshold) & ((heights<1.40)|(material==2))
        material_lines |= valid & (((material==2)&(other_material==1))|((material==3)&np.isin(other_material,[1,2])))
    remaining = lines.copy()
    supported = np.zeros_like(lines)
    for row, column in np.argwhere(lines):
        if not remaining[row,column]:
            continue
        component, pending = [], [(row,column)]
        remaining[row,column] = False
        while pending:
            current_row, current_column = pending.pop()
            component.append((current_row,current_column))
            for offset_row, offset_column in ((-1,0),(1,0),(0,-1),(0,1),(-1,-1),(-1,1),(1,-1),(1,1)):
                neighbour_row, neighbour_column = current_row+offset_row,current_column+offset_column
                if 0 <= neighbour_row < lines.shape[0] and 0 <= neighbour_column < lines.shape[1] and remaining[neighbour_row,neighbour_column]:
                    remaining[neighbour_row,neighbour_column]=False
                    pending.append((neighbour_row,neighbour_column))
        if len(component) >= FINISH["minimum_line_cluster"]:
            for component_row, component_column in component:
                supported[component_row,component_column]=True
    return supported | material_lines


def resolve_frame(path, width, height, *, geometry_path=None):
    source = Image.open(path).convert("RGBA")
    if source.size != (width*4,height*4):
        raise ValueError(f"Unexpected walk render size: {path}: {source.size}")
    pixels = np.array(source.resize((width,height),Image.Resampling.BOX))
    opaque = pixels[:,:,3]>=CONFIG["alpha_threshold"]
    material, depth, heights = load_geometry(path,width,height,geometry_path)
    rgb = remove_isolated_colors(quantize_materials(pixels[:,:,:3],material),opaque,material,heights)
    outer = np.logical_or.reduce(neighbours(opaque)) & ~opaque
    inner = internal_lines(material,depth,heights,opaque)
    # Darken the lower/right lip without swallowing fingers in a double outline.
    broad = sum(neighbours(opaque)) >= 7
    shadow_rim = opaque & broad & (~shifted(opaque,-1,0)|~shifted(opaque,0,-1))
    rgb[shadow_rim & (material==1)] = ImageColor.getrgb(FINISH["skin_shadow_rim"])
    rgb[shadow_rim & (material!=1)] = ImageColor.getrgb(FINISH["inner_ink"])
    rgb[inner] = ImageColor.getrgb(FINISH["inner_ink"])
    rgb[inner & (material==2)] = ImageColor.getrgb(FINISH["outer_ink"])
    rgb[outer] = ImageColor.getrgb(FINISH["outer_ink"])
    result = np.zeros((height,width,4),dtype=np.uint8)
    mask = opaque | outer
    result[mask,:3] = rgb[mask]
    result[mask,3] = 255
    if not mask.any() or mask[0].any() or mask[-1].any() or mask[:,0].any() or mask[:,-1].any():
        raise ValueError(f"Empty or clipped ink sprite: {path}")
    image = Image.fromarray(result)
    stats = {"opaque_bbox":list(image.getbbox()),"colors_used":len(np.unique(result[mask,:3],axis=0)),"alpha_values":sorted(np.unique(result[:,:,3]).tolist()),"outer_ink_pixels":int(outer.sum()),"internal_ink_pixels":int(inner.sum())}
    return image, stats
