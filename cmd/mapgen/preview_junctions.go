package main

func junctionPreviewGroup(river RiverOptions) previewGroup {
	chance := floatField("junction_chance", 0, 1, 0.01, river.JunctionChance)
	chance.Label = "Вероятность Y-соединения"
	chance = describedField(chance, "Одна попытка выбора Y на подходящую дополнительную связь: 0 выключает, 1 всегда пробует. Каркас и основные выходы к границе не меняются. При тесноте остаётся обычная связь; это не доля всех рек. Требует Option B и фарватер.")
	spacing := intField("junction_spacing_tiles", 0, 8192, river.JunctionSpacingTiles)
	spacing.Label = "Расстояние между Y, тайлы"
	spacing = describedField(spacing, "Минимальное расстояние между Y; при включённой вероятности должно быть > 0. Между Y и притоком применяется максимум этого значения, tributary_spacing_tiles и физической ширины русел.")
	return previewGroup{Title: "Y-соединения рек", Fields: []previewField{chance, spacing}}
}
