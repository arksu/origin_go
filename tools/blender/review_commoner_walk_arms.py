"""Compare the saved faulty binding and the corrected W/E pixel animations."""
import sys
from pathlib import Path

sys.dont_write_bytecode=True
sys.path.insert(0,str(Path(__file__).resolve().parent))
from PIL import Image, ImageDraw
from bake_male_commoner_walk import OUT, save_gif, font, paste_sprite

frames=[]
for index in range(8):
    canvas=Image.new("RGB",(840,920),"#202b30")
    draw=ImageDraw.Draw(canvas)
    draw.text((24,18),"РУКИ / ИСПРАВЛЕНИЕ ПРИВЯЗКИ К СКЕЛЕТУ",font=font(25),fill="#f7dfb8")
    draw.text((158,65),"ДО",font=font(23),fill="#a2b6b7")
    draw.text((536,65),"ИСПРАВЛЕНО",font=font(23),fill="#f7dfb8")
    for row,direction in enumerate(("w","e")):
        bottom=486+row*420
        draw.text((22,bottom-206),direction.upper(),font=font(24),fill="#a2b6b7")
        for column,folder in enumerate((OUT/"arm_review/before_pixel",OUT/"pixel/8")):
            sprite=Image.open(folder/direction/f"{index:02}.png").convert("RGBA")
            paste_sprite(canvas,sprite,210+column*420,bottom,4)
    frames.append(canvas)
save_gif(frames,OUT/"previews/walk_arms_before_after.gif",[120]*8)
frames[4].save(OUT/"previews/walk_arms_before_after.png")
print("ARM_REVIEW_COMPLETE")
