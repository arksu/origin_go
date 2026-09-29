package main

func describedField(field previewField, description string) previewField {
	field.Description = description
	return field
}

func boolField(key string, value bool, description string) previewField {
	return previewField{Key: key, Label: key, Type: "bool", Default: value, Description: description}
}

func unboundedField(field previewField, description string) previewField {
	field.RangeMax, field.Max = field.Max, nil
	return describedField(field, description)
}

func worldPreviewSchema(base MapgenOptions) previewLayerSchema {
	return previewLayerSchema{
		Name: previewParamWorld, Title: "World", ParamKey: previewParamWorld,
		Groups: []previewGroup{{Title: "Рельеф и вода Перлина", Fields: []previewField{
			unboundedField(floatField("terrain_scale", 0, 0.02, 0.0001, base.TerrainScale), "Базовая частота рельефа и климата. Больше — мельче детали. Должна быть > 0; воду отключает отдельный флаг."),
			boolField("perlin_water_enabled", base.PerlinWaterEnabled, "Вода в низинах шума высоты. false убирает её, сохраняя рисуемые реки и озёра речной сети."),
		}}},
	}
}

func biomesLayerSchema(base MapgenOptions) previewLayerSchema {
	biome := base.Biome
	groups := []previewGroup{
		{Title: "Включение", Fields: []previewField{
			boolField("enabled", biome.Enabled, "Включить генератор биомов. false использует базовую классификацию земли; это отличается от скрытия слоя."),
			boolField("blob_enabled", biome.BlobEnabled, "Органические пятна, детали и очистка. false оставляет траву, горы, камень и климатический песок."),
		}},
		{Title: "Климат", Fields: []previewField{
			unboundedField(floatField("temperature_scale", 0, 4, 0.01, biome.TemperatureScale), "Частота температуры, > 0. Больше — мельче температурные области."),
			unboundedField(floatField("moisture_scale", 0, 4, 0.01, biome.MoistureScale), "Частота влажности, > 0. Больше — мельче влажные и сухие области."),
			unboundedField(floatField("continentalness_scale", 0, 4, 0.01, biome.ContinentalnessScale), "Частота континентальности, > 0; влияет на мокроту и климатический песок."),
			unboundedField(floatField("erosion_scale", 0, 4, 0.01, biome.ErosionScale), "Частота эрозии, > 0. Поле вычисляется, но сейчас не влияет на выбор биома."),
			unboundedField(floatField("weirdness_scale", 0, 4, 0.01, biome.WeirdnessScale), "Частота вариативности, > 0. Поле вычисляется, но сейчас не влияет на выбор биома."),
			describedField(floatField("domain_warp_strength", 0, 512, 1, biome.DomainWarpStrength), "Искажение координат климатических полей. 0 отключает искажение."),
			describedField(floatField("hnh_swamp_clump_scale", 0, 4, 0.01, biome.SwampClumpScale), "Множитель мокроты для болот и глины; допустимо (0, 4]."),
		}},
		{Title: "Горы и камень", Fields: []previewField{
			describedField(floatField("hnh_mountain_rugged_threshold", 0, 1, 0.01, biome.MountainRuggedThreshold), "Порог горного поля: меньше — больше гор."),
			describedField(floatField("mountain_massif_scale", 1, 4096, 1, biome.MountainMassifScale), "Масштаб горных массивов в тайлах, независимо от других биомов."),
			describedField(floatField("mountain_stone_scale", 1, 4096, 1, biome.MountainStoneScale), "Масштаб каменных участков в горах; не больше mountain_massif_scale."),
		}},
		{Title: "Размещение пятен", Fields: []previewField{
			unboundedField(intField("blob_seed_spacing", 1, 2048, biome.BlobSeedSpacing), "Шаг сетки основных пятен в тайлах. Меньше — больше кандидатов."),
			describedField(floatField("blob_seed_jitter", 0, 1, 0.01, biome.BlobSeedJitter), "Смещение центров внутри ячеек, [0, 1). 0 оставляет регулярную сетку."),
			unboundedField(intField("blob_secondary_spacing", 1, 1024, biome.BlobSecondarySpacing), "Шаг сетки кустарника, земли и глины в тайлах."),
		}},
		{Title: "Веса основных биомов", Fields: []previewField{
			unboundedField(floatField("blob_forest_weight", 0, 10, 0.01, biome.BlobForestWeight), "Относительный вес леса. 0 исключает лес из выбора основных пятен."),
			unboundedField(floatField("blob_heath_weight", 0, 10, 0.01, biome.BlobHeathWeight), "Относительный вес вересковой области. 0 исключает этот вариант выбора."),
			unboundedField(floatField("blob_moor_weight", 0, 10, 0.01, biome.BlobMoorWeight), "Относительный вес пустоши с учётом климата. 0 исключает этот вариант выбора."),
			unboundedField(floatField("blob_swamp_weight", 0, 10, 0.01, biome.BlobSwampWeight), "Относительный вес болота при подходящей мокроте. 0 исключает болота."),
			unboundedField(floatField("blob_skip_weight", 0, 10, 0.01, biome.BlobSkipWeight), "Вес пропуска кандидата. 0 отключает случайный пропуск; неподходящие места всё равно пропускаются."),
		}},
		{Title: "Плотность мелких пятен", Fields: []previewField{
			describedField(floatField("blob_thicket_density", 0, 1, 0.01, biome.BlobThicketDensity), "Вероятность кустарника в ячейке леса, а не доля площади. 0 отключает."),
			describedField(floatField("blob_dirt_density", 0, 1, 0.01, biome.BlobDirtDensity), "Вероятность земли на траве. 0 отключает; сумма с blob_clay_density должна быть <= 1."),
			describedField(floatField("blob_clay_density", 0, 1, 0.01, biome.BlobClayDensity), "Вероятность глины на подходящей траве. 0 отключает; сумма с blob_dirt_density должна быть <= 1."),
		}},
		{Title: "Климатические пороги", Fields: []previewField{
			describedField(floatField("blob_forest_cold_threshold", 0, 1, 0.01, biome.BlobForestColdThreshold), "Ниже этой температуры лес хвойный, иначе лиственный."),
			describedField(floatField("blob_moor_moisture_max", 0, 1, 0.01, biome.BlobMoorMoistureMax), "Пустошь допустима при влажности ниже порога или при достаточно низкой температуре."),
			describedField(floatField("blob_moor_temperature_max", 0, 1, 0.01, biome.BlobMoorTemperatureMax), "Пустошь допустима при температуре ниже порога или при достаточно низкой влажности."),
			describedField(floatField("blob_swamp_wetness_min", 0, 1, 0.01, biome.BlobSwampWetnessMin), "Болото допустимо при мокроте строго выше порога или при достаточной влажности."),
			describedField(floatField("blob_swamp_moisture_min", 0, 1, 0.01, biome.BlobSwampMoistureMin), "Болото допустимо при влажности строго выше порога или при достаточной мокроте."),
			describedField(floatField("blob_clay_wetness_min", 0, 1, 0.01, biome.BlobClayWetnessMin), "Глина допустима при мокроте строго выше порога или при достаточной влажности."),
			describedField(floatField("blob_clay_moisture_min", 0, 1, 0.01, biome.BlobClayMoistureMin), "Глина допустима при влажности строго выше порога или при достаточной мокроте."),
		}},
	}
	for _, shape := range biome.blobShapes() {
		prefix := "blob_" + shape.name
		groups = append(groups, previewGroup{Title: "Размеры: " + shape.name, Fields: []previewField{
			describedField(floatField(prefix+"_size_min", 1, 4096, 1, shape.minSize), "Минимальная длина ветви пятна в тайлах, не радиус. Не больше size_max."),
			describedField(floatField(prefix+"_size_max", 1, 4096, 1, shape.maxSize), "Максимальная длина ветви пятна в тайлах. Не меньше size_min."),
			describedField(floatField(prefix+"_width", 0, 4096, 1, shape.width), "Толщина контура независимо от длины ветвей. 0 выбирает половину случайной длины ветви."),
		}})
	}
	groups = append(groups,
		previewGroup{Title: "Форма и ветвление", Fields: []previewField{
			describedField(floatField("blob_raggedness", 0, 64, 0.1, biome.BlobRaggedness), "Максимальное смещение контура по каждой координате. 0 отключает неровность."),
			describedField(intField("blob_max_nodes", 4, 256, biome.BlobMaxNodes), "Предел узлов скелета пятна; ограничивает сложность и расход памяти."),
			describedField(intField("blob_max_depth", 3, 32, biome.BlobMaxDepth), "Предел глубины ветвления скелета пятна."),
		}},
		previewGroup{Title: "Мелкие островки", Fields: []previewField{
			describedField(floatField("blob_islet_chance", 0, 1, 0.01, biome.BlobIsletChance), "Вероятность отдельного пятнышка у края основного биома. 0 отключает."),
			unboundedField(intField("blob_islet_spacing", 1, 1024, biome.BlobIsletSpacing), "Шаг проверки периметра для островков, > 0. Меньше — больше попыток размещения."),
		}},
		previewGroup{Title: "Сглаживание и очистка", Fields: []previewField{
			unboundedField(intField("hnh_smoothing_passes", 0, 10, biome.SmoothingPasses), "Число проходов сглаживания основных пятен. 0 отключает сглаживание."),
			unboundedField(intField("hnh_min_patch_tiles", 1, 1024, biome.MinPatchTiles), "Минимальная площадь основного пятна. 1 отключает удаление мелких пятен; детали и островки добавляются позже."),
		}},
	)
	return previewLayerSchema{Name: previewParamBiomes, Title: "Biomes", ParamKey: previewParamBiomes, Groups: groups}
}
