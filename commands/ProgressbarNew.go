package commands

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v5"
	"github.com/vbauerster/mpb/v5/decor"
)

// Data for the bar proxy
type barData struct {
	// Offset represents n bytes which
	// are written to the server but not
	// to the progressbar
	offset int
}

// BarTask for the bar to do
type BarTask uint8

// ...
const (
	UploadTask BarTask = iota
	DownloadTask
)

// Implement string
func (bt BarTask) String() string {
	switch bt {
	case UploadTask:
		return "Upload"
	case DownloadTask:
		return "Download"
	}

	return ""
}

// Verb return task as verb
func (bt BarTask) Verb() string {
	return bt.String() + "ing"
}

// Bar a porgressbar
type Bar struct {
	task    BarTask
	total   int64
	options []mpb.BarOption
	style   string

	bar *mpb.Bar

	// Original writer for the proxy
	ow io.Writer

	// Data required for the proxy
	barData *barData

	doneTextChan chan string
	doneText     string
	done         bool
}

// NewBar create a new bar
func NewBar(task BarTask, total int64, name string) *Bar {
	// Create bar instance
	bar := &Bar{
		task:         task,
		total:        total,
		style:        "(=>_)",
		barData:      &barData{},
		doneTextChan: make(chan string, 1),
	}

	// Trim name
	if len(name) > 40 {
		name = name[:20] + "..." + name[len(name)-20:]
	}

	// Add Bar options
	bar.options = append([]mpb.BarOption{}, mpb.BarFillerMiddleware(func(base mpb.BarFiller) mpb.BarFiller {
		return mpb.BarFillerFunc(func(w io.Writer, reqWidth int, st decor.Statistics) {
			if bar.done {
				io.WriteString(w, bar.doneText)
				return
			}

			// Check if there is text in the doneText channel
			select {
			case text := <-bar.doneTextChan:
				bar.doneText = text
				bar.done = true
				io.WriteString(w, text)
				fmt.Println(bar.bar.Completed())
				return
			default:
			}

			base.Fill(w, reqWidth, st)
		})
	}))

	bar.options = append(bar.options, []mpb.BarOption{
		mpb.BarFillerClearOnComplete(),
		mpb.PrependDecorators(
			decor.OnComplete(decor.Spinner(nil, decor.WCSyncSpace), "done"),
			decor.Name(task.Verb(), decor.WCSyncSpace),
			decor.Name(" '"+name+"'", decor.WCSyncSpaceR),
			decor.Percentage(decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			decor.CountersKiloByte("[%d / %d]", decor.WCSyncSpace),
		),
	}...)

	return bar
}

// Implement the io.Writer for the bar proxy
func (bar Bar) Write(b []byte) (int, error) {
	n, err := bar.ow.Write(b)

	// if bar is set, write to it
	if bar.bar != nil {
		// If cached writtenBytes are
		// not restored yet, restore them
		if bar.barData.offset > 0 {
			bar.bar.IncrBy(bar.barData.offset)
			bar.barData.offset = 0
		}

		bar.bar.IncrBy(n)
	} else {
		// If bar is not visible yet,
		// cache written bytes
		bar.barData.offset += n
	}

	return n, err
}

// ProgressView holds info for progress
type ProgressView struct {
	ProgressContainer *mpb.Progress
	Bars              []*mpb.Bar
}

// AddBar to ProgressView
func (pv *ProgressView) AddBar(bbar *Bar) *mpb.Bar {
	// Add bar to render queue
	bar := pv.ProgressContainer.Add(bbar.total, mpb.NewBarFiller(bbar.style, false), bbar.options...)

	// Set Bars mpb.Bar to allow it
	// to increase
	bbar.bar = bar

	// Append bar to pv bars
	pv.Bars = append(pv.Bars, bar)

	// Return prepared proxy func
	return bar
}

// NewProgressView create new progressview
func NewProgressView() *ProgressView {
	return &ProgressView{
		Bars: []*mpb.Bar{},
		ProgressContainer: mpb.New(
			mpb.WithWaitGroup(&sync.WaitGroup{}),
			mpb.WithRefreshRate(50*time.Millisecond),
			mpb.WithWidth(130),
		),
	}
}
