package main

func lakePreviewField(field previewField, label, description string) previewField {
	field.Label = label
	return describedField(field, description)
}

func lakeDetailPreviewGroup(river RiverOptions) previewGroup {
	return previewGroup{Title: "Берега озёр и острова", Fields: []previewField{
		lakePreviewField(boolField("lake_irregular_enabled", river.LakeIrregularEnabled, ""), "Берега C", "Заливы, полуострова и рваный берег. Требует layout_draw и положительную ширину фарватера; false возвращает прежнюю форму озёр."),
		lakePreviewField(intField("lake_peninsula_count_max", 0, 8, river.LakePeninsulaCountMax), "Полуострова: максимум", "Максимум полуостровов на озеро. Неудачные кандидаты пропускаются; 0 отключает полуострова."),
		lakePreviewField(floatField("lake_peninsula_depth_ratio", 0, 0.6, 0.01, river.LakePeninsulaDepthRatio), "Глубина полуостровов", "Предельное проникновение суши как доля локального радиуса озера; 0 отключает вырезы."),
		lakePreviewField(intField("lake_shore_variation_tiles", 0, 16, river.LakeShoreVariationTiles), "Неровность берега", "Максимальное смещение береговой линии, тайлы; 0 убирает мелкую рваность."),
		lakePreviewField(floatField("lake_island_small_chance", 0, 1, 0.01, river.LakeIslandSmallChance), "Остров: малое озеро", "Вероятность попытки один раз на малое озеро, от 0 до 1; 0 отключает. Место проверяется отдельно."),
		lakePreviewField(floatField("lake_island_medium_chance", 0, 1, 0.01, river.LakeIslandMediumChance), "Остров: среднее озеро", "Вероятность попытки одного острова на среднее озеро. Фактическая частота может быть ниже из-за тесноты."),
		lakePreviewField(floatField("lake_island_large_chance", 0, 1, 0.01, river.LakeIslandLargeChance), "Остров: большое озеро", "Вероятность выбора большого озера для островов. Все три вероятности 0 полностью отключают острова."),
		lakePreviewField(floatField("lake_island_second_chance", 0, 1, 0.01, river.LakeIslandSecondChance), "Второй остров", "Условная вероятность двух островов в уже выбранном большом озере; 0 означает максимум один."),
		lakePreviewField(intField("lake_island_radius_min", 2, 64, river.LakeIslandRadiusMin), "Радиус острова: минимум", "Минимальный номинальный радиус сухой части, тайлы. Мелководье добавляется снаружи; слишком тесный кандидат пропускается."),
		lakePreviewField(intField("lake_island_radius_max", 2, 64, river.LakeIslandRadiusMax), "Радиус острова: максимум", "Максимальный номинальный радиус сухой части; не меньше min. Суммарная суша островов ограничена 8% площади озера."),
		lakePreviewField(intField("lake_shallow_width_min", 0, 32, river.LakeShallowWidthMin), "Мелководье озёр: минимум", "Минимальная полоса вдоль берегов озера и островов, тайлы. Настройки мелководья рек задаются отдельно."),
		lakePreviewField(intField("lake_shallow_width_max", 0, 32, river.LakeShallowWidthMax), "Мелководье озёр: максимум", "Максимальная полоса, >= min. Равные границы дают постоянную ширину; обе 0 отключают полосу. Глубокие проходы сохраняются."),
	}}
}
