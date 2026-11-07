package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strconv"
	"strings"
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

	input := bufio.NewReader(os.Stdin)
	var lambda float64
	fmt.Print("Введите lambda (например, 10000): ")
	if _, err := fmt.Fscanln(input, &lambda); err != nil || lambda < 0 || math.IsNaN(lambda) || math.IsInf(lambda, 0) {
		fmt.Fprintln(os.Stderr, "Lambda должна быть конечным неотрицательным числом")
		os.Exit(1)
	}

	var photoCount int
	fmt.Print("Количество снимков (минимум 3): ")
	if _, err := fmt.Fscanln(input, &photoCount); err != nil || photoCount < 3 {
		fmt.Fprintln(os.Stderr, "Введите целое число не меньше 3")
		os.Exit(1)
	}

	var images []image.Image
	var logExposureTimes []float64
	var bounds image.Rectangle
	for j := 0; j < photoCount; j++ {
		fmt.Printf("Путь к снимку %d: ", j+1)
		path, _ := input.ReadString('\n')
		img, err := loadImage(strings.Trim(strings.TrimSpace(path), "\""))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if j == 0 {
			bounds = img.Bounds()
		} else if img.Bounds() != bounds {
			fmt.Fprintln(os.Stderr, "Размеры изображений не совпадают")
			os.Exit(1)
		}

		fmt.Printf("Выдержка снимка %d в секундах (например, 1/750 или 0.001333): ", j+1)
		var exposure string
		_, err = fmt.Fscanln(input, &exposure)
		t, valid := parseExposure(exposure)
		if err != nil || !valid {
			fmt.Fprintln(os.Stderr, "Выдержка должна быть положительным конечным числом")
			os.Exit(1)
		}
		images = append(images, img)
		logExposureTimes = append(logExposureTimes, math.Log(t))
	}

	width, height := bounds.Dx(), bounds.Dy()

	var gridSize int
	maxGridSize := min(width, height)
	fmt.Printf("Введите размер сетки (от 1 до %d, например 16): ", maxGridSize)
	if _, err := fmt.Fscanln(input, &gridSize); err != nil || gridSize < 1 || gridSize > maxGridSize {
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

	redSamples := make([][]uint8, len(points))
	greenSamples := make([][]uint8, len(points))
	blueSamples := make([][]uint8, len(points))

	for i, point := range points {
		redSamples[i] = make([]uint8, photoCount)
		greenSamples[i] = make([]uint8, photoCount)
		blueSamples[i] = make([]uint8, photoCount)
		for j, img := range images {
			r, g, b := getRGB(img, point.X, point.Y)

			redSamples[i][j] = r
			greenSamples[i][j] = g
			blueSamples[i][j] = b
		}
	}

	gRed := solveResponse(redSamples, logExposureTimes, lambda)
	gGreen := solveResponse(greenSamples, logExposureTimes, lambda)
	gBlue := solveResponse(blueSamples, logExposureTimes, lambda)

	//fmt.Println(gRed, "\n", gGreen, "\n", gBlue, "\n")

	var scale float64
	fmt.Print("Введите scale (для примера из input попробуйте 0.001–0.003, например 0.0015): ")
	if _, err := fmt.Fscanln(input, &scale); err != nil || scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		fmt.Fprintln(os.Stderr, "Scale должен быть положительным конечным числом")
		os.Exit(1)
	}

	red := make([]uint8, photoCount)
	green := make([]uint8, photoCount)
	blue := make([]uint8, photoCount)
	hdr := make([][3]float64, width*height)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
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

	if err := os.MkdirAll("output", 0755); err != nil {
		panic(err)
	}
	if err := saveHDR("output/result.hdr", hdr, width, height); err != nil {
		panic(err)
	}

	out := image.NewRGBA(bounds)

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

func parseExposure(text string) (float64, bool) {
	numerator, denominator, fraction := strings.Cut(text, "/")
	t, err := strconv.ParseFloat(numerator, 64)
	if err != nil || t <= 0 || math.IsInf(t, 0) || math.IsNaN(t) {
		return 0, false
	}
	if fraction {
		divisor, err := strconv.ParseFloat(denominator, 64)
		if err != nil || divisor <= 0 || math.IsInf(divisor, 0) || math.IsNaN(divisor) {
			return 0, false
		}
		t /= divisor
	}
	return t, t > 0 && !math.IsInf(t, 0)
}
