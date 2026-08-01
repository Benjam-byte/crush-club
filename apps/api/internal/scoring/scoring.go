package scoring

import "math"

type AnswerScore struct {
	BaseScore     int
	MaximumScore  int
	Exact         bool
	LoverSelected bool
}

func FinalScore(answer AnswerScore) int {
	if !answer.LoverSelected {
		return answer.BaseScore
	}
	if answer.Exact {
		return answer.MaximumScore * 2
	}
	return answer.BaseScore - answer.MaximumScore
}

func IntegerRangeScore(official, predicted, minValue, maxValue, maximumScore int) int {
	if maxValue <= minValue || maximumScore < 0 {
		return 0
	}
	distance := math.Abs(float64(official - predicted))
	span := float64(maxValue - minValue)
	score := math.Round(float64(maximumScore) * (1 - distance/span))
	if score < 0 {
		return 0
	}
	if score > float64(maximumScore) {
		return maximumScore
	}
	return int(score)
}

func SingleChoiceScore(official, predicted string, maximumScore int) int {
	if maximumScore < 0 {
		return 0
	}
	if official != predicted {
		return 0
	}
	return maximumScore
}

func NumberScore(official, predicted, floor float64, maximumScore int) int {
	if maximumScore < 0 {
		return 0
	}

	denominator := math.Max(math.Abs(official), floor)
	if denominator <= 0 {
		if official == predicted {
			return maximumScore
		}
		return 0
	}

	relativeDistance := math.Abs(predicted-official) / denominator
	switch {
	case relativeDistance <= 0.10:
		return maximumScore
	case relativeDistance <= 0.25:
		return roundedPercentage(maximumScore, 0.75)
	case relativeDistance <= 0.50:
		return roundedPercentage(maximumScore, 0.50)
	case relativeDistance <= 0.75:
		return roundedPercentage(maximumScore, 0.25)
	default:
		return 0
	}
}

func MultiChoiceScore(official, predicted []string, maximumScore int) int {
	if maximumScore < 0 {
		return 0
	}

	officialSet := createStringSet(official)
	predictedSet := createStringSet(predicted)
	unionSet := make(map[string]struct{}, len(officialSet)+len(predictedSet))
	intersectionCount := 0

	for option := range officialSet {
		unionSet[option] = struct{}{}
		if _, exists := predictedSet[option]; exists {
			intersectionCount++
		}
	}
	for option := range predictedSet {
		unionSet[option] = struct{}{}
	}

	if len(unionSet) == 0 {
		return maximumScore
	}

	score := math.Round(float64(maximumScore) * float64(intersectionCount) / float64(len(unionSet)))
	return int(score)
}

func RankedChoiceScore(official, predicted []string) int {
	positionPointList := []int{6, 4, 3}
	positionCount := min(len(positionPointList), len(official), len(predicted))
	score := 0
	isExact := len(official) == len(predicted)

	for position := 0; position < positionCount; position++ {
		if official[position] == predicted[position] {
			score += positionPointList[position]
			continue
		}

		isExact = false
		if containsString(official, predicted[position]) {
			score += positionPointList[position] / 2
		}
	}

	if isExact {
		score += 2
	}
	if score > 15 {
		return 15
	}
	return score
}

func roundedPercentage(maximumScore int, percentage float64) int {
	return int(math.Round(float64(maximumScore) * percentage))
}

func createStringSet(optionList []string) map[string]struct{} {
	optionSet := make(map[string]struct{}, len(optionList))
	for _, option := range optionList {
		optionSet[option] = struct{}{}
	}
	return optionSet
}

func containsString(optionList []string, target string) bool {
	for _, option := range optionList {
		if option == target {
			return true
		}
	}
	return false
}
