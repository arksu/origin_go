"""Compare pixel finishes on unchanged terrain and prop textures from the game."""
from __future__ import annotations

import json
import sys
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0,str(Path(__file__).resolve().parent))
from PIL import Image, ImageColor, ImageDraw
from bake_male_commoner_v3_pixel import resolve_frame as original_frame, CONFIG as ORIGINAL_CONFIG, font
from commoner_pixel_ink import CONFIG, OUT

ROOT=Path(__file__).resolve().parents[2]
GAME=ROOT/"web_new/public/assets/game"


def scenery(terrain):
    atlas=Image.open(GAME/"tiles.png").convert("RGBA")
    entries=json.loads((GAME/"tiles.json").read_text())["frames"]
    tiles=[]
    for number in (1,2,3):
        bounds=entries[f"tiles/{terrain}/base_{number}.png"]["frame"]
        tiles.append(atlas.crop((bounds["x"],bounds["y"],bounds["x"]+bounds["w"],bounds["y"]+bounds["h"])))
    canvas=Image.new("RGB",(160,128),"#394325")
    for row in range(-2,9):
        for column in range(-2,4):
            tile=tiles[(row+column*2)%3]
            canvas.paste(tile,(column*62+(row%2)*31,row*16),tile)
    props=[("obj/barrel/barrel.png",(24,99)),("obj/trees/log/log_y.png",(133,119))]
    for name,(center,bottom) in props:
        prop=Image.open(GAME/name).convert("RGBA")
        prop=prop.crop(prop.getbbox())
        canvas.paste(prop,(center-prop.width//2,bottom-prop.height),prop)
    return canvas


def review():
    backgrounds=[scenery("grass"),scenery("dirt")]
    frames=[]
    for index in range(8):
        board=Image.new("RGB",(1000,904),"#202b30")
        draw=ImageDraw.Draw(board)
        draw.text((25,16),"ПРОВЕРКА НА ТЕКСТУРАХ ИЗ ИГРЫ",font=font(27),fill="#f7dfb8")
        draw.text((25,55),"Одна анимация · 80 × 96 px · увеличение 3×",font=font(18),fill="#a2b6b7")
        draw.text((200,90),"ДО",font=font(24),fill="#f7dfb8")
        draw.text((660,90),"С ОБВОДКОЙ",font=font(24),fill="#f7dfb8")
        for row,direction in enumerate(("se","sw")):
            before,_=original_frame(OUT/"raw/8"/direction/f"{index:02}.png",80,96)
            after=Image.open(OUT/"pixel/8"/direction/f"{index:02}.png").convert("RGBA")
            for column,sprite in enumerate((before,after)):
                canvas=backgrounds[row].copy()
                canvas.paste(sprite,(80-40,110-96),sprite)
                canvas=canvas.resize((480,384),Image.Resampling.NEAREST)
                board.paste(canvas,(10+column*500,126+row*388))
        frames.append(board)
    reserved=list(dict.fromkeys(ORIGINAL_CONFIG["palette"]+CONFIG["palette"]+["#202b30","#f7dfb8","#a2b6b7"]))
    texture_palette=frames[0].quantize(colors=256-len(reserved),dither=Image.Dither.NONE).getpalette()
    palette=[channel for color in reserved for channel in ImageColor.getrgb(color)] + texture_palette[:(256-len(reserved))*3]
    palette_image=Image.new("P",(1,1)); palette_image.putpalette(palette)
    converted=[frame.quantize(palette=palette_image,dither=Image.Dither.NONE) for frame in frames]
    converted[0].save(OUT/"previews/walk_ink_before_after.gif",save_all=True,append_images=converted[1:],duration=120,loop=0,disposal=2,optimize=False)
    frames[0].save(OUT/"previews/walk_ink_before_after.png")
    print("INK_CONTEXT_REVIEW_COMPLETE")


if __name__=="__main__":
    review()
