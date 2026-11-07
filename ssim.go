package main

import (
	"fmt"
	"image"
	"math"
)

// calculateSSIM сравнивает изображения после одинакового тонального отображения.
// Окно 11x11, sigma=1.5, K1=0.01, K2=0.03, L=255.
func calculateSSIM(first, second image.Image) (float64, error) {
	if first == nil || second == nil {
		return 0, fmt.Errorf("SSIM: изображение не задано")
	}
	bounds := first.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if bounds.Size() != second.Bounds().Size() {
		return 0, fmt.Errorf("SSIM: размеры изображений различаются")
	}
	const windowSize = 11
	const sigma = 1.5
	if width < windowSize || height < windowSize {
		return 0, fmt.Errorf("SSIM: минимальный размер изображения 11x11")
	}

	// Перевод в градации серого 0–255; прозрачность учитывается на чёрном фоне.
	toGray := func(img image.Image) []float64 {
		gray := make([]float64, width*height)
		start := img.Bounds().Min
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, g, b, _ := img.At(start.X+x, start.Y+y).RGBA()
				gray[y*width+x] = math.Round((0.299*float64(r) +
					0.587*float64(g) + 0.114*float64(b)) / 257)
			}
		}
		return gray
	}
	grayA, grayB := toGray(first), toGray(second)

	// Нормировка гауссовых весов: их сумма равна единице
	// Используем гауссовы веса: пиксели ближе к центру окна влияют сильнее и нормируем веса, чтобы их сумма равнялась 1
	var weights [windowSize * windowSize]float64
	var weightSum float64
	for y := 0; y < windowSize; y++ {
		for x := 0; x < windowSize; x++ {
			dx, dy := float64(x-windowSize/2), float64(y-windowSize/2)
			w := math.Exp(-(dx*dx + dy*dy) / (2 * sigma * sigma))
			weights[y*windowSize+x] = w
			weightSum += w
		}
	}
	for i := range weights {
		weights[i] /= weightSum
	}
	const c1 = (0.01 * 255) * (0.01 * 255)
	const c2 = (0.03 * 255) * (0.03 * 255)
	var total float64
	var count int

	// Проходим по всем окнам изображения, используем формулу SSIM для вычисления индекса сходства
	for y := 0; y <= height-windowSize; y++ {
		for x := 0; x <= width-windowSize; x++ {
			var meanA, meanB, squareA, squareB, product float64
			for wy := 0; wy < windowSize; wy++ {
				base := (y+wy)*width + x
				for wx := 0; wx < windowSize; wx++ {
					w := weights[wy*windowSize+wx]
					a, b := grayA[base+wx], grayB[base+wx]
					// Вычисляем среднее, дисперсию и ковариацию с использованием весов
					meanA += w * a
					meanB += w * b
					squareA += w * a * a
					squareB += w * b * b
					product += w * a * b
				}
			}
			// Защита от отрицательной дисперсии из-за округления
			// Дисперсия показывает насколько сильно внутри различаются значения серого
			varianceA := math.Max(0, squareA-meanA*meanA)
			varianceB := math.Max(0, squareB-meanB*meanB)
			// Ковариация показывает совпадают ли светлые и тёмные детали по расположению
			covariance := product - meanA*meanB
			numerator := (2*meanA*meanB + c1) * (2*covariance + c2)
			denominator := (meanA*meanA + meanB*meanB + c1) *
				(varianceA + varianceB + c2)
			total += numerator / denominator
			count++
		}
	}
	return total / float64(count), nil
}

// runSSIM читает два изображения и выводит SSIM без повторного построения HDR
func runSSIM(paths []string) error {
	if len(paths) != 2 {
		return fmt.Errorf("использование: go run . -ssim first.png second.png")
	}
	first, err := loadImage(paths[0])
	if err != nil {
		return fmt.Errorf("SSIM: чтение %q: %w", paths[0], err)
	}
	second, err := loadImage(paths[1])
	if err != nil {
		return fmt.Errorf("SSIM: чтение %q: %w", paths[1], err)
	}
	score, err := calculateSSIM(first, second)
	if err != nil {
		return err
	}
	fmt.Printf("SSIM = %.9f\n", score)
	fmt.Printf("Размер: %dx%d; окно: 11x11; sigma: 1.5; градации серого\n",
		first.Bounds().Dx(), first.Bounds().Dy())
	fmt.Println("Сравниваются изображения после одинакового tone mapping и гамма-коррекции")
	return nil
}
