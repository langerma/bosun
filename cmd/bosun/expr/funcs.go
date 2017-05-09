package expr

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"bosun.org/cmd/bosun/expr/doc"
	"bosun.org/cmd/bosun/expr/parse"
	"bosun.org/models"
	"bosun.org/opentsdb"
	"github.com/GaryBoone/GoStats/stats"
	"github.com/MiniProfiler/go/miniprofiler"
	"github.com/jinzhu/now"
)

func init() {
	for _, v := range reductionFuncs {
		err := v.Doc.SetCodeLink(v.F)
		if err != nil {
			log.Fatal(err)
		}
	}
	docs := doc.Docs{
		"reduction": reductionFuncs.DocSlice(),
		"group":     groupFuncs.DocSlice(),
		"time":      timeFuncs.DocSlice(),
		"builtins":  builtins.DocSlice(),
	}
	b, err := docs.Wiki()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(b.String())
	os.Exit(0)
	// ds, err := json.Marshal(docs)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println(string(ds))
	// os.Exit(0)
}

func tagQuery(args []parse.Node) (parse.Tags, error) {
	n := args[0].(*parse.StringNode)
	// Since all 2.1 queries are valid 2.2 queries, at this time
	// we can just use 2.2 to parse to identify group by tags
	q, err := opentsdb.ParseQuery(n.Text, opentsdb.Version2_2)
	if q == nil && err != nil {
		return nil, err
	}
	t := make(parse.Tags)
	for k := range q.GroupByTags {
		t[k] = struct{}{}
	}
	return t, nil
}

func tagFirst(args []parse.Node) (parse.Tags, error) {
	return args[0].Tags()
}

func seriesFuncTags(args []parse.Node) (parse.Tags, error) {
	t := make(parse.Tags)
	text := args[0].(*parse.StringNode).Text
	if text == "" {
		return t, nil
	}
	ts, err := opentsdb.ParseTags(text)
	if err != nil {
		return nil, err
	}

	for k := range ts {
		t[k] = struct{}{}
	}
	return t, nil
}

var builtins = parse.FuncMap{
	// Other functions

	"abs": {
		Args:   []models.FuncType{models.TypeNumberSet},
		Return: models.TypeNumberSet,
		Tags:   tagFirst,
		F:      Abs,
	},
	"crop": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberSet, models.TypeNumberSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      Crop,
	},

	"des": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeScalar, models.TypeScalar},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      Des,
	},
	"dropge": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      DropGe,
	},
	"dropg": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      DropG,
	},
	"drople": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      DropLe,
	},
	"dropl": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      DropL,
	},
	"dropna": {
		Args:   []models.FuncType{models.TypeSeriesSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      DropNA,
	},
	"dropbool": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeSeriesSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      DropBool,
	},
	"filter": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      Filter,
	},
	"limit": {
		Args:   []models.FuncType{models.TypeNumberSet, models.TypeScalar},
		Return: models.TypeNumberSet,
		Tags:   tagFirst,
		F:      Limit,
	},
	"linelr": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeString},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      Line_lr,
	},
	"nv": {
		Args:   []models.FuncType{models.TypeNumberSet, models.TypeScalar},
		Return: models.TypeNumberSet,
		Tags:   tagFirst,
		F:      NV,
	},
	"series": {
		Args:      []models.FuncType{models.TypeString, models.TypeScalar},
		VArgs:     true,
		VArgsPos:  1,
		VArgsOmit: true,
		Return:    models.TypeSeriesSet,
		Tags:      seriesFuncTags,
		F:         SeriesFunc,
	},
	"sort": {
		Args:   []models.FuncType{models.TypeNumberSet, models.TypeString},
		Return: models.TypeNumberSet,
		Tags:   tagFirst,
		F:      Sort,
	},
	"leftjoin": {
		Args:     []models.FuncType{models.TypeString, models.TypeString, models.TypeNumberSet},
		VArgs:    true,
		VArgsPos: 2,
		Return:   models.TypeTable,
		Tags:     nil, // TODO
		F:        LeftJoin,
	},
	"merge": {
		Args:   []models.FuncType{models.TypeSeriesSet},
		VArgs:  true,
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      Merge,
	},
	"tail": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberSet},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      Tail,
	},
	"map": {
		Args:   []models.FuncType{models.TypeSeriesSet, models.TypeNumberExpr},
		Return: models.TypeSeriesSet,
		Tags:   tagFirst,
		F:      Map,
	},
	"v": {
		Return:  models.TypeScalar,
		F:       V,
		MapFunc: true,
	},
}

func V(e *State, T miniprofiler.Timer) (*Results, error) {
	return fromScalar(e.vValue), nil
}

func Map(e *State, T miniprofiler.Timer, series *Results, expr *Results) (*Results, error) {
	newExpr := Expr{expr.Results[0].Value.Value().(NumberExpr).Tree}
	for _, result := range series.Results {
		newSeries := make(Series)
		for t, v := range result.Value.Value().(Series) {
			e.vValue = v
			subResults, _, err := newExpr.ExecuteState(e, T)
			if err != nil {
				return series, err
			}
			for _, res := range subResults.Results {
				var v float64
				switch res.Value.Value().(type) {
				case Number:
					v = float64(res.Value.Value().(Number))
				case Scalar:
					v = float64(res.Value.Value().(Scalar))
				default:
					return series, fmt.Errorf("wrong return type for map expr: %v", res.Type())
				}
				newSeries[t] = v
			}
		}
		result.Value = newSeries
	}
	return series, nil
}

func SeriesFunc(e *State, T miniprofiler.Timer, tags string, pairs ...float64) (*Results, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("uneven number of time stamps and values")
	}
	group := opentsdb.TagSet{}
	if tags != "" {
		var err error
		group, err = opentsdb.ParseTags(tags)
		if err != nil {
			return nil, fmt.Errorf("unable to parse tags: %v", err)
		}
	}

	series := make(Series)
	for i := 0; i < len(pairs); i += 2 {
		series[time.Unix(int64(pairs[i]), 0)] = pairs[i+1]
	}
	return &Results{
		Results: []*Result{
			{
				Value: series,
				Group: group,
			},
		},
	}, nil
}

func Crop(e *State, T miniprofiler.Timer, sSet *Results, startSet *Results, endSet *Results) (*Results, error) {
	results := Results{}
INNER:
	for _, seriesResult := range sSet.Results {
		for _, startResult := range startSet.Results {
			for _, endResult := range endSet.Results {
				startHasNoGroup := len(startResult.Group) == 0
				endHasNoGroup := len(endResult.Group) == 0
				startOverlapsSeries := seriesResult.Group.Overlaps(startResult.Group)
				endOverlapsSeries := seriesResult.Group.Overlaps(endResult.Group)
				if (startHasNoGroup || startOverlapsSeries) && (endHasNoGroup || endOverlapsSeries) {
					res := crop(e, seriesResult, startResult, endResult)
					results.Results = append(results.Results, res)
					continue INNER
				}
			}
		}
	}
	return &results, nil
}

func crop(e *State, seriesResult *Result, startResult *Result, endResult *Result) *Result {
	startNumber := startResult.Value.(Number)
	endNumber := endResult.Value.(Number)
	start := e.now.Add(-time.Duration(time.Duration(startNumber) * time.Second))
	end := e.now.Add(-time.Duration(time.Duration(endNumber) * time.Second))
	series := seriesResult.Value.(Series)
	for timeStamp := range series {
		if timeStamp.Before(start) || timeStamp.After(end) {
			delete(series, timeStamp)
		}
	}
	return seriesResult
}

func DropBool(e *State, T miniprofiler.Timer, target *Results, filter *Results) (*Results, error) {
	res := Results{}
	unions := e.union(target, filter, "dropbool union")
	for _, union := range unions {
		aSeries := union.A.Value().(Series)
		bSeries := union.B.Value().(Series)
		newSeries := make(Series)
		for k, v := range aSeries {
			if bv, ok := bSeries[k]; ok {
				if bv != float64(0) {
					newSeries[k] = v
				}
			}
		}
		if len(newSeries) > 0 {
			res.Results = append(res.Results, &Result{Group: union.Group, Value: newSeries})
		}
	}
	return &res, nil
}

func Epoch(e *State, T miniprofiler.Timer) (*Results, error) {
	return &Results{
		Results: []*Result{
			{Value: Scalar(float64(e.now.Unix()))},
		},
	}, nil
}

func Month(e *State, T miniprofiler.Timer, offset float64, startEnd string) (*Results, error) {
	if startEnd != "start" && startEnd != "end" {
		return nil, fmt.Errorf("last parameter for mtod must be 'start' or 'end'")
	}
	offsetInt := int(offset)
	location := time.FixedZone(fmt.Sprintf("%v", offsetInt), offsetInt*60*60)
	timeZoned := e.now.In(location)
	var mtod float64
	if startEnd == "start" {
		mtod = float64(now.New(timeZoned).BeginningOfMonth().Unix())
	} else {
		mtod = float64(now.New(timeZoned).EndOfMonth().Unix())
	}
	return &Results{
		Results: []*Result{
			{Value: Scalar(float64(mtod))},
		},
	}, nil
}

func NV(e *State, T miniprofiler.Timer, series *Results, v float64) (results *Results, err error) {
	// If there are no results in the set, promote it to a number with the empty group ({})
	if len(series.Results) == 0 {
		series.Results = append(series.Results, &Result{Value: Number(v), Group: make(opentsdb.TagSet)})
		return series, nil
	}
	series.NaNValue = &v
	return series, nil
}

func Sort(e *State, T miniprofiler.Timer, series *Results, order string) (*Results, error) {
	// Sort by groupname first to make the search deterministic
	sort.Sort(ResultSliceByGroup(series.Results))
	switch order {
	case "desc":
		sort.Stable(sort.Reverse(ResultSliceByValue(series.Results)))
	case "asc":
		sort.Stable(ResultSliceByValue(series.Results))
	default:
		return nil, fmt.Errorf("second argument of order() must be asc or desc")
	}
	return series, nil
}

func Limit(e *State, T miniprofiler.Timer, series *Results, v float64) (*Results, error) {
	i := int(v)
	if len(series.Results) > i {
		series.Results = series.Results[:i]
	}
	return series, nil
}

func Filter(e *State, T miniprofiler.Timer, series *Results, number *Results) (*Results, error) {
	var ns ResultSlice
	for _, sr := range series.Results {
		for _, nr := range number.Results {
			if sr.Group.Subset(nr.Group) || nr.Group.Subset(sr.Group) {
				if nr.Value.Value().(Number) != 0 {
					ns = append(ns, sr)
				}
			}
		}
	}
	series.Results = ns
	return series, nil
}

func Tail(e *State, T miniprofiler.Timer, series *Results, number *Results) (*Results, error) {
	f := func(res *Results, s *Result, floats []float64) error {
		tailLength := int(floats[0])

		// if there are fewer points than the requested tail
		// short circut and just return current series
		if len(s.Value.Value().(Series)) <= tailLength {
			res.Results = append(res.Results, s)
			return nil
		}

		// create new sorted series
		// not going to do quick select
		// see https://github.com/bosun-monitor/bosun/pull/1802
		// for details
		oldSr := s.Value.Value().(Series)
		sorted := NewSortedSeries(oldSr)

		// create new series keep a reference
		// and point sr.Value interface at reference
		// as we don't need old series any more
		newSeries := make(Series)
		s.Value = newSeries

		// load up new series with desired
		// number of points
		// we already checked len so this is safe
		for _, item := range sorted[len(sorted)-tailLength:] {
			newSeries[item.T] = item.V
		}
		res.Results = append(res.Results, s)
		return nil
	}

	return match(f, series, number)
}

func Merge(e *State, T miniprofiler.Timer, series ...*Results) (*Results, error) {
	res := &Results{}
	if len(series) == 0 {
		return res, fmt.Errorf("merge requires at least one result")
	}
	if len(series) == 1 {
		return series[0], nil
	}
	seen := make(map[string]bool)
	for _, r := range series {
		for _, entry := range r.Results {
			if _, ok := seen[entry.Group.String()]; ok {
				return res, fmt.Errorf("duplicate group in merge: %s", entry.Group.String())
			}
			seen[entry.Group.String()] = true
		}
		res.Results = append(res.Results, r.Results...)
	}
	return res, nil
}

func LeftJoin(e *State, T miniprofiler.Timer, keysCSV, columnsCSV string, rowData ...*Results) (*Results, error) {
	res := &Results{}
	dataWidth := len(rowData)
	if dataWidth == 0 {
		return res, fmt.Errorf("leftjoin requires at least one item to populate rows")
	}
	keyColumns := strings.Split(keysCSV, ",")
	dataColumns := strings.Split(columnsCSV, ",")
	if len(dataColumns) != dataWidth {
		return res, fmt.Errorf("mismatch in length of data rows and data labels")
	}
	keyWidth := len(keyColumns)
	keyIndex := make(map[string]int, keyWidth)
	for i, v := range keyColumns {
		keyIndex[v] = i
	}
	t := Table{}
	t.Columns = append(keyColumns, dataColumns...)
	rowWidth := len(dataColumns) + len(keyColumns)
	rowGroups := []opentsdb.TagSet{}
	for i, r := range rowData {
		if i == 0 {
			for _, val := range r.Results {
				row := make([]interface{}, rowWidth)
				for k, v := range val.Group {
					if ki, ok := keyIndex[k]; ok {
						row[ki] = v
					}
				}
				row[keyWidth+i] = val.Value.Value()
				rowGroups = append(rowGroups, val.Group)
				t.Rows = append(t.Rows, row)
			}
			continue
		}
		for rowIndex, group := range rowGroups {
			for _, val := range r.Results {
				if group.Subset(val.Group) {
					t.Rows[rowIndex][keyWidth+i] = val.Value.Value()
				}
			}
		}
	}
	return &Results{
		Results: []*Result{
			{Value: t},
		},
	}, nil
}




func ToDuration(e *State, T miniprofiler.Timer, sec float64) (*Results, error) {
	d := opentsdb.Duration(time.Duration(int64(sec)) * time.Second)
	return &Results{
		Results: []*Result{
			{Value: String(d.HumanString())},
		},
	}, nil
}

func DropValues(e *State, T miniprofiler.Timer, series *Results, threshold *Results, dropFunction func(float64, float64) bool) (*Results, error) {
	f := func(res *Results, s *Result, floats []float64) error {
		nv := make(Series)
		for k, v := range s.Value.Value().(Series) {
			if !dropFunction(float64(v), floats[0]) {
				//preserve values which should not be discarded
				nv[k] = v
			}
		}
		if len(nv) == 0 {
			return fmt.Errorf("series %s is empty", s.Group)
		}
		s.Value = nv
		res.Results = append(res.Results, s)
		return nil
	}
	return match(f, series, threshold)
}

func DropGe(e *State, T miniprofiler.Timer, series *Results, threshold *Results) (*Results, error) {
	dropFunction := func(value float64, threshold float64) bool { return value >= threshold }
	return DropValues(e, T, series, threshold, dropFunction)
}

func DropG(e *State, T miniprofiler.Timer, series *Results, threshold *Results) (*Results, error) {
	dropFunction := func(value float64, threshold float64) bool { return value > threshold }
	return DropValues(e, T, series, threshold, dropFunction)
}

func DropLe(e *State, T miniprofiler.Timer, series *Results, threshold *Results) (*Results, error) {
	dropFunction := func(value float64, threshold float64) bool { return value <= threshold }
	return DropValues(e, T, series, threshold, dropFunction)
}

func DropL(e *State, T miniprofiler.Timer, series *Results, threshold *Results) (*Results, error) {
	dropFunction := func(value float64, threshold float64) bool { return value < threshold }
	return DropValues(e, T, series, threshold, dropFunction)
}

func DropNA(e *State, T miniprofiler.Timer, series *Results) (*Results, error) {
	dropFunction := func(value float64, threshold float64) bool {
		return math.IsNaN(float64(value)) || math.IsInf(float64(value), 0)
	}
	return DropValues(e, T, series, fromScalar(0), dropFunction)
}

func fromScalar(f float64) *Results {
	return &Results{
		Results: ResultSlice{
			&Result{
				Value: Number(f),
			},
		},
	}
}

func match(f func(res *Results, series *Result, floats []float64) error, series *Results, numberSets ...*Results) (*Results, error) {
	res := *series
	res.Results = nil
	for _, s := range series.Results {
		var floats []float64
		for _, num := range numberSets {
			for _, n := range num.Results {
				if len(n.Group) == 0 || s.Group.Overlaps(n.Group) {
					floats = append(floats, float64(n.Value.(Number)))
					break
				}
			}
		}
		if len(floats) != len(numberSets) {
			if !series.IgnoreUnjoined {
				return nil, fmt.Errorf("unjoined groups for %s", s.Group)
			}
			continue
		}
		if err := f(&res, s, floats); err != nil {
			return nil, err
		}
	}
	return &res, nil
}

func Abs(e *State, T miniprofiler.Timer, series *Results) *Results {
	for _, s := range series.Results {
		s.Value = Number(math.Abs(float64(s.Value.Value().(Number))))
	}
	return series
}

func TimeDelta(e *State, T miniprofiler.Timer, series *Results) (*Results, error) {
	for _, res := range series.Results {
		sorted := NewSortedSeries(res.Value.Value().(Series))
		newSeries := make(Series)
		if len(sorted) < 2 {
			newSeries[sorted[0].T] = 0
			res.Value = newSeries
			continue
		}
		lastTime := sorted[0].T.Unix()
		for _, dp := range sorted[1:] {
			unixTime := dp.T.Unix()
			diff := unixTime - lastTime
			newSeries[dp.T] = float64(diff)
			lastTime = unixTime
		}
		res.Value = newSeries
	}
	return series, nil
}

func Count(e *State, T miniprofiler.Timer, query, sduration, eduration string) (r *Results, err error) {
	r, err = Query(e, T, query, sduration, eduration)
	if err != nil {
		return
	}
	return &Results{
		Results: []*Result{
			{Value: Scalar(len(r.Results))},
		},
	}, nil
}

func Des(e *State, T miniprofiler.Timer, series *Results, alpha float64, beta float64) *Results {
	for _, res := range series.Results {
		sorted := NewSortedSeries(res.Value.Value().(Series))
		if len(sorted) < 2 {
			continue
		}
		des := make(Series)
		s := make([]float64, len(sorted))
		b := make([]float64, len(sorted))
		s[0] = sorted[0].V
		for i := 1; i < len(sorted); i++ {
			s[i] = alpha*sorted[i].V + (1-alpha)*(s[i-1]+b[i-1])
			b[i] = beta*(s[i]-s[i-1]) + (1-beta)*b[i-1]
			des[sorted[i].T] = s[i]
		}
		res.Value = des
	}
	return series
}

func Line_lr(e *State, T miniprofiler.Timer, series *Results, d string) (*Results, error) {
	dur, err := opentsdb.ParseDuration(d)
	if err != nil {
		return series, err
	}
	for _, res := range series.Results {
		res.Value = line_lr(res.Value.(Series), time.Duration(dur))
		res.Group.Merge(opentsdb.TagSet{"regression": "line"})
	}
	return series, nil
}

// line_lr generates a series representing the line up to duration in the future.
func line_lr(dps Series, d time.Duration) Series {
	var x []float64
	var y []float64
	sortedDPS := NewSortedSeries(dps)
	var maxT time.Time
	if len(sortedDPS) > 1 {
		maxT = sortedDPS[len(sortedDPS)-1].T
	}
	for _, v := range sortedDPS {
		xv := float64(v.T.Unix())
		x = append(x, xv)
		y = append(y, v.V)
	}
	var slope, intercept, _, _, _, _ = stats.LinearRegression(x, y)
	s := make(Series)
	// First point in the regression line
	s[maxT] = float64(maxT.Unix())*slope + intercept
	// Last point
	last := maxT.Add(d)
	s[last] = float64(last.Unix())*slope + intercept
	return s
}

var sSeriesSetArg = doc.Arg{
	Name: "s",
	Type: models.TypeSeriesSet,
}

var nNumberSetArg = doc.Arg{
	Name: "n",
	Type: models.TypeNumberSet,
}
