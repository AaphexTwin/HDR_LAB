package main

import (
	//"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

func weight(z uint8) float64 {
	if z <= 127 {
		return float64(z)
	}
	return float64(255 - z)
}

func solveResponse(samples [][]uint8, logExposureTimes []float64, lambda float64) []float64 {
	var rows [][]float64
	var rightSides []float64

	for i, pixelSamples := range samples {
		for j, z := range pixelSamples {
			w := weight(z)
			if w == 0 {
				continue // полностью чёрное или пересвеченное измерение
			}

			coefficients := make([]float64, 256+len(samples))
			coefficients[int(z)] = w
			coefficients[256+i] = -w

			rows = append(rows, coefficients)
			rightSides = append(rightSides, w*logExposureTimes[j])
		}
	}
	// якорь выбирает общий сдвиг при котором g(128)=0 сохраняя возможность выполнить остальные уравнения
	anchor := make([]float64, 256+len(samples))
	anchor[128] = 1 // точка отсчета масштаба света, строка коэф
	rows = append(rows, anchor)
	rightSides = append(rightSides, 0)

	// Штрафуем резкие изменения наклона g между соседними значениями z.
	for z := 1; z < 255; z++ {
		scale := math.Sqrt(lambda) * weight(uint8(z)) // усиление строки плавности
		coefficients := make([]float64, 256+len(samples))
		coefficients[z-1] = scale
		coefficients[z] = -2 * scale
		coefficients[z+1] = scale

		rows = append(rows, coefficients)
		rightSides = append(rightSides, 0)
	}

	nRows := len(rows)
	nCols := 256 + len(samples)

	matrixA := mat.NewDense(nRows, nCols, nil)
	for i, row := range rows {
		for j, value := range row {
			matrixA.Set(i, j, value)
		}
	}

	bVec := mat.NewVecDense(nRows, rightSides)

	var svd mat.SVD

	if !svd.Factorize(matrixA, mat.SVDThin) { // SVDThin выбирает компактную форму хранения разложения
		panic("can not to solve")
	}

	// Порог для определения ранга матрицы
	rank := svd.Rank(1e-12)
	if rank == 0 {
		panic("rank = 0")
	}

	var solution mat.VecDense
	svd.SolveVecTo(&solution, bVec, rank)

	curve := make([]float64, 256)
	for z := range curve {
		curve[z] = solution.AtVec(z)
	}

	return curve
}

func estimateLogE(values []uint8, curve []float64, logTimes []float64) float64 {
	var sum, sumWeights float64
	for j, z := range values {
		w := weight(z)
		sum += w * (curve[z] - logTimes[j])
		sumWeights += w
	}

	if sumWeights == 0 {
		middle := len(values) / 2
		return curve[values[middle]] - logTimes[middle]
	}
	return sum / sumWeights
}
