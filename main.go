package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func toneMap(e, scale float64) uint8 {
	value := 1 - math.Exp(-scale*e)  // сжатие HDR-диапазона
	value = math.Pow(value, 1.0/2.2) // гамма-коррекция (кодирование яркости для экрана)
	return uint8(math.Round(255 * value))
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-ssim" {
		if err := runSSIM(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	var lambda float64
	fmt.Print("Введите lambda (например, 10000): ")
	if _, err := fmt.Scanln(&lambda); err != nil || lambda < 0 || math.IsNaN(lambda) || math.IsInf(lambda, 0) {
		fmt.Fprintln(os.Stderr, "Lambda должна быть конечным неотрицательным числом")
		os.Exit(1)
	}

	img1, err := loadImage("input/dark.jpg")
	if err != nil {
		panic(err)
	}

	img2, err := loadImage("input/normal.jpg")
	if err != nil {
		panic(err)
	}

	img3, err := loadImage("input/bright.jpg")
	if err != nil {
		panic(err)
	}

	fmt.Println(img1.Bounds())
	fmt.Println(img2.Bounds())
	fmt.Println(img3.Bounds())

	if img1.Bounds() != img2.Bounds() || img1.Bounds() != img3.Bounds() {
		panic("Размеры изображений не совпадают")
	}

	bounds := img1.Bounds()

	width := bounds.Dx()
	height := bounds.Dy()

	exposureTimes := []float64{
		1.0 / 750.0,
		1.0 / 180.0,
		1.0 / 45.0,
	}

	images := []image.Image{img1, img2, img3}

	var gridSize int
	maxGridSize := min(width, height)
	fmt.Printf("Введите размер сетки (от 1 до %d, например 16): ", maxGridSize)
	if _, err := fmt.Scanln(&gridSize); err != nil || gridSize < 1 || gridSize > maxGridSize {
		fmt.Fprintf(os.Stderr, "Размер сетки должен быть целым числом от 1 до %d\n", maxGridSize)
		os.Exit(1)
	}

	points := make([]image.Point, 0, gridSize*gridSize)

	for row := 0; row < gridSize; row++ {
		for col := 0; col < gridSize; col++ {
			x := bounds.Min.X + (2*col+1)*width/(2*gridSize) // работа с целыми числами (без 0.5)
			y := bounds.Min.Y + (2*row+1)*height/(2*gridSize)

			points = append(points, image.Pt(x, y))
		}
	}

	//fmt.Println("Количество точек:", len(points))

	redSamples := make([][3]uint8, len(points))
	greenSamples := make([][3]uint8, len(points))
	blueSamples := make([][3]uint8, len(points))

	for i, point := range points {
		for j, img := range images {
			r, g, b := getRGB(img, point.X, point.Y)

			redSamples[i][j] = r
			greenSamples[i][j] = g
			blueSamples[i][j] = b
		}
	}

	logExposureTimes := make([]float64, len(exposureTimes))
	for j, t := range exposureTimes {
		logExposureTimes[j] = math.Log(t)
	}

	//fmt.Println("Logariphms: ", logExposureTimes)

	gRed := solveResponse(redSamples, logExposureTimes, lambda)
	gGreen := solveResponse(greenSamples, logExposureTimes, lambda)
	gBlue := solveResponse(blueSamples, logExposureTimes, lambda)

	//fmt.Println(gRed, "\n", gGreen, "\n", gBlue, "\n")

	hdr := make([][3]float64, width*height)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var red, green, blue [3]uint8

			// Одна и та же точка на трёх фотографиях
			for j, img := range images {
				red[j], green[j], blue[j] = getRGB(img, x, y)
			}

			// Оценка света отдельно для каждого цвета.
			lnR := estimateLogE(red, gRed, logExposureTimes)
			lnG := estimateLogE(green, gGreen, logExposureTimes)
			lnB := estimateLogE(blue, gBlue, logExposureTimes)

			index := (y-bounds.Min.Y)*width + (x - bounds.Min.X)
			hdr[index] = [3]float64{
				math.Exp(lnR),
				math.Exp(lnG),
				math.Exp(lnB),
			}
		}
	}

	if err := saveHDR("output/result.hdr", hdr, width, height); err != nil {
		panic(err)
	}

	out := image.NewRGBA(bounds)
	scale := 0.0015 // пробное значение яркости

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			index := (y-bounds.Min.Y)*width + (x - bounds.Min.X)
			pixel := hdr[index]

			out.SetRGBA(x, y, color.RGBA{
				R: toneMap(pixel[0], scale),
				G: toneMap(pixel[1], scale),
				B: toneMap(pixel[2], scale),
				A: 255,
			})
		}
	}

	file, err := os.Create("result.png")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if err := png.Encode(file, out); err != nil {
		panic(err)
	}

}
