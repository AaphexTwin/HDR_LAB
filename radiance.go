package main

import (
	"fmt"
	"math"
	"os"
)

func saveHDR(path string, hdr [][3]float64, width, height int) error {
	if width <= 0 || height <= 0 || len(hdr) != width*height {
		return fmt.Errorf("размеры HDR не совпадают с количеством пикселей")
	}

	// Заголовок и размеры изображения
	header := fmt.Sprintf(
		"#?RADIANCE\nFORMAT=32-bit_rle_rgbe\n\n-Y %d +X %d\n",
		height, width,
	)

	// После заголовка  по четыре байта RGBE на пиксель
	data := make([]byte, len(header)+4*len(hdr))
	copy(data, header)

	for i, pixel := range hdr {
		for _, value := range pixel {
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("некорректное HDR-значение в пикселе %d", i)
			}
		}

		maximum := math.Max(pixel[0], math.Max(pixel[1], pixel[2]))
		if maximum < 1e-32 {
			continue // Этот пиксель остаётся чёрным: четыре нулевых байта
		}

		// Представляем maximum как m * 2^exponent
		// Сохраняем только показатель степени
		_, exponent := math.Frexp(maximum)

		if exponent > 127 {
			return fmt.Errorf("пиксель %d слишком яркий для RGBE", i)
		}

		// Вычисляем множитель 2^(8-exponent), чтобы максимальная
		// компонента попала в диапазон [128, 256), а остальные были не больше неё
		factor := math.Ldexp(1, 8-exponent)

		// Находим начало четырёх байтов текущего пикселя после заголовка
		offset := len(header) + 4*i

		// Масштабируем три канала и записываем их в байты
		// При преобразовании в byte дробная часть отбрасывается
		data[offset] = byte(pixel[0] * factor)
		data[offset+1] = byte(pixel[1] * factor)
		data[offset+2] = byte(pixel[2] * factor)

		// Записываем общий показатель степени со смещением 128
		// чтобы можно было хранить и отрицательные показатели
		data[offset+3] = byte(exponent + 128)
	}

	return os.WriteFile(path, data, 0644)
}
