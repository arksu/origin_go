"""Apply a reproducible skin-only material correction inside Blender.

The source image remains packed separately; repeated exports never compound the
adjustment. Clothing, buckle and hair UV samples form a protected mask.
"""
from pathlib import Path
import bpy
import numpy as np

SKIN_BRIGHTNESS = 1.15


def triangle_pixels(image_size, triangle):
    coordinates = np.asarray(triangle) * (image_size - 1)
    lower = np.maximum(np.floor(coordinates.min(axis=0)).astype(int), 0)
    upper = np.minimum(np.ceil(coordinates.max(axis=0)).astype(int), image_size - 1)
    columns, rows = np.meshgrid(np.arange(lower[0], upper[0] + 1), np.arange(lower[1], upper[1] + 1))
    points = np.stack([columns, rows], axis=-1)
    first, second, third = coordinates
    determinant = np.cross(second - first, third - first)
    if abs(determinant) < 1e-9:
        return rows[:0], columns[:0]
    first_weight = np.cross(second - points, third - points) / determinant
    second_weight = np.cross(third - points, first - points) / determinant
    inside = (first_weight >= -.001) & (second_weight >= -.001) & (first_weight + second_weight <= 1.001)
    return rows[inside], columns[inside]


def brighten_skin(mesh, output_directory):
    material = mesh.data.materials[0]
    texture = next(node for node in material.node_tree.nodes if node.type == 'TEX_IMAGE')
    source_name = material.get('skin_brightness_source')
    source = bpy.data.images.get(source_name) if source_name else texture.image
    if source is None:
        raise RuntimeError('Original Meshy base color is unavailable')
    material['skin_brightness_source'] = source.name
    source.use_fake_user = True
    width, height = source.size
    if width != height:
        raise RuntimeError('Expected square Meshy UV atlas')
    original = np.asarray(source.pixels[:], dtype=np.float32).reshape(height, width, 4)
    red, green, blue = original[:, :, :3].transpose(2, 0, 1)
    # The atlas has warm light skin and dark leather. Keep a conservative color
    # boundary, then explicitly protect garment/hair UV faces, including highlights.
    skin = (red > .42) & (green > .26) & (blue > .15) & (red - green > .10) & (green - blue > .055)
    protected = np.zeros((height, width), dtype=bool)
    mesh.data.calc_loop_triangles()
    uv = mesh.data.uv_layers.active.data
    for triangle in mesh.data.loop_triangles:
        coordinates = [tuple(uv[loop].uv) for loop in triangle.loops]
        center = sum((mesh.data.vertices[index].co for index in triangle.vertices), mesh.data.vertices[0].co * 0) / 3
        sample_uv = np.mean(coordinates, axis=0)
        column, row = np.clip(np.rint(sample_uv * (width - 1)).astype(int), 0, width - 1)
        color = original[row, column, :3]
        garment = abs(center.x) < .32 and .69 < center.z < 1.21 and color[0] < .43
        buckle = abs(center.x) < .105 and 1.04 < center.z < 1.19 and center.y < -.10
        hair = center.z > 1.67 and color[0] < .47
        if garment or buckle or hair:
            rows, columns = triangle_pixels(width, coordinates)
            protected[rows, columns] = True
    # Protect texture-filter footprints at material boundaries, too.
    padded = np.pad(protected, 2)
    protected = np.logical_or.reduce([padded[row:row + height, column:column + width] for row in range(5) for column in range(5)])
    skin &= ~protected
    adjusted = original.copy()
    adjusted[skin, :3] = np.minimum(original[skin, :3] * SKIN_BRIGHTNESS, 1)
    if np.any(adjusted[protected] != original[protected]):
        raise RuntimeError('Protected clothing or hair pixels changed')
    image = bpy.data.images.get('Meshy skin +15 percent')
    if image is None:
        image = bpy.data.images.new('Meshy skin +15 percent', width, height, alpha=True)
    image.colorspace_settings.name = source.colorspace_settings.name
    image.pixels.foreach_set(adjusted.ravel())
    image.filepath_raw = str(Path(output_directory) / 'skin-brightened.png')
    image.file_format = 'PNG'
    image.save()
    image.pack()
    texture.image = image
    return {'skin_brightness': SKIN_BRIGHTNESS, 'adjusted_texels': int(skin.sum()), 'protected_texels': int(protected.sum()), 'protected_max_difference': 0}
