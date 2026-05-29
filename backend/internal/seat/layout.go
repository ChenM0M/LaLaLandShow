package seat

type Definition struct {
	Zone        string
	RowNo       int
	ColNo       int
	IsBlocked   bool
	BlockReason string
}

func DefaultLayout() []Definition {
	defs := make([]Definition, 0, 385)
	defs = appendZone(defs, "left", []int{8, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7})
	defs = appendZone(defs, "middle", repeat(25, 8))
	defs = appendZone(defs, "right", []int{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 2})

	for i := range defs {
		if (defs[i].Zone == "middle" && defs[i].RowNo <= 2) ||
			(defs[i].Zone == "left" && defs[i].RowNo == 1) {
			defs[i].IsBlocked = true
			defs[i].BlockReason = "default_reserved"
		}
	}

	return defs
}

func appendZone(defs []Definition, zone string, rowWidths []int) []Definition {
	for rowIndex, width := range rowWidths {
		rowNo := rowIndex + 1
		for colNo := 1; colNo <= width; colNo++ {
			defs = append(defs, Definition{
				Zone:  zone,
				RowNo: rowNo,
				ColNo: colNo,
			})
		}
	}
	return defs
}

func repeat(count, value int) []int {
	values := make([]int, count)
	for i := range values {
		values[i] = value
	}
	return values
}
