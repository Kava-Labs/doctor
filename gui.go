// gui.go contains types, functions and methods for creating
// and updating the graphical and interactive UI to the doctor program

package main

import (
	"fmt"
	"log"
	"math"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/spf13/viper"
)

// GUIConfig wraps values
// used to configure the GUI
// display mode of the doctor program
type GUIConfig struct {
	DebugLoggingEnabled bool
}

// GUI controls the display
// mode of the doctor program
// using asci interactive tty
// output devices
type GUI struct {
	grid            *ui.Grid
	updateParagraph func(count int)
	draw            func(count int, paragraph string)
	*log.Logger
}

// Watch watches for new measurements and log messages for the kava node with the
// specified rpc api url, outputting them to the gui device in the desired format
func (g *GUI) Watch(metricReadOnlyChannels MetricReadOnlyChannels, logMessages <-chan string, kavaNodeRPCURL string) error {
	tickerCount := 1

	// create channel to subscribe to
	// user input

	uiEvents := ui.PollEvents()

	// create channel that will emit
	// an event every second
	ticker := time.NewTicker(time.Second).C

	for {
		select {
		// events triggered by user input
		// or action such as keyboard strokes
		// mouse movements or window changes
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				ui.Close()
				return nil
			case "c":
				updatedParagraph := fmt.Sprintf(`
				Current Config %+v
				`, viper.AllSettings())
				g.draw(tickerCount, updatedParagraph)
				time.Sleep(1 * time.Second)
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				g.grid.SetRect(0, 0, payload.Width, payload.Height)
				ui.Clear()
				ui.Render(g.grid)
			}
		// events triggered by new data
		case syncStatusMetrics := <-metricReadOnlyChannels.SyncStatusMetrics:
			updatedParagraph := fmt.Sprintf(`
			Latest Block Height %d
			Sync Status Latency (milliseconds) %d
			`, syncStatusMetrics.SyncStatus.LatestBlockHeight, syncStatusMetrics.MeasurementLatencyMilliseconds)
			g.draw(tickerCount, updatedParagraph)
		case logMessage := <-logMessages:
			// TODO: separate channels
			// for debug only log messages?
			// if !debugMode {
			// 	continue
			// }
			g.draw(tickerCount, logMessage)
			time.Sleep(2 * time.Second)
		// events triggered on a regular time based interval
		case <-ticker:
			g.updateParagraph(tickerCount)
			g.draw(tickerCount, "")
			tickerCount++
		}
	}
}

func NewGUI(config GUIConfig) (*GUI, error) {
	if err := ui.Init(); err != nil {
		panic(fmt.Errorf("failed to initialize termui: %v", err))
	}

	// uppper left box
	p := widgets.NewParagraph()
	p.Title = "Text Box"
	p.Text = "PRESS q TO QUIT DEMO"
	p.SetRect(0, 0, 50, 5)
	p.TextStyle.Fg = ui.ColorWhite
	p.BorderStyle.Fg = ui.ColorCyan

	updateParagraph := func(count int) {
		if count%2 == 0 {
			p.TextStyle.Fg = ui.ColorRed
		} else {
			p.TextStyle.Fg = ui.ColorWhite
		}
	}

	// upper right box
	listData := []string{
		"[0] gizak/termui",
		"[1] editbox.go",
		"[2] interrupt.go",
		"[3] keyboard.go",
		"[4] output.go",
		"[5] random_out.go",
		"[6] dashboard.go",
		"[7] nsf/termbox-go",
	}

	// lower left box
	l := widgets.NewList()
	l.Title = "List"
	l.Rows = listData
	l.SetRect(0, 5, 25, 12)
	l.TextStyle.Fg = ui.ColorYellow

	g := widgets.NewGauge()
	g.Title = "Gauge"
	g.Percent = 50
	g.SetRect(0, 12, 50, 15)
	g.BarColor = ui.ColorRed
	g.BorderStyle.Fg = ui.ColorWhite
	g.TitleStyle.Fg = ui.ColorCyan

	sparklineData := []float64{4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6, 4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6, 4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6, 4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6}

	sl := widgets.NewSparkline()
	sl.Title = "srv 0:"
	sl.Data = sparklineData
	sl.LineColor = ui.ColorCyan
	sl.TitleStyle.Fg = ui.ColorWhite

	sl2 := widgets.NewSparkline()
	sl2.Title = "srv 1:"
	sl2.Data = sparklineData
	sl2.TitleStyle.Fg = ui.ColorWhite
	sl2.LineColor = ui.ColorRed

	slg := widgets.NewSparklineGroup(sl, sl2)
	slg.Title = "Sparkline"
	slg.SetRect(25, 5, 50, 12)

	sinData := (func() []float64 {
		n := 220
		ps := make([]float64, n)
		for i := range ps {
			ps[i] = 1 + math.Sin(float64(i)/5)
		}
		return ps
	})()

	lc := widgets.NewPlot()
	lc.Title = "dot-marker Line Chart"
	lc.Data = make([][]float64, 1)
	lc.Data[0] = sinData
	lc.SetRect(0, 15, 50, 25)
	lc.AxesColor = ui.ColorWhite
	lc.LineColors[0] = ui.ColorRed
	lc.Marker = widgets.MarkerDot

	barchartData := []float64{3, 2, 5, 3, 9, 5, 3, 2, 5, 8, 3, 2, 4, 5, 3, 2, 5, 7, 5, 3, 2, 6, 7, 4, 6, 3, 6, 7, 8, 3, 6, 4, 5, 3, 2, 4, 6, 4, 8, 5, 9, 4, 3, 6, 5, 3, 6}

	bc := widgets.NewBarChart()
	bc.Title = "Bar Chart"
	bc.SetRect(50, 0, 75, 10)
	bc.Labels = []string{"S0", "S1", "S2", "S3", "S4", "S5"}
	bc.BarColors[0] = ui.ColorGreen
	bc.NumStyles[0] = ui.NewStyle(ui.ColorBlack)

	// lower right box
	lc2 := widgets.NewPlot()
	lc2.Title = "braille-mode Line Chart"
	lc2.Data = make([][]float64, 1)
	lc2.Data[0] = sinData
	lc2.SetRect(50, 15, 75, 25)
	lc2.AxesColor = ui.ColorWhite
	lc2.LineColors[0] = ui.ColorYellow

	// set up 4 x 4 grid
	grid := ui.NewGrid()
	termWidth, termHeight := ui.TerminalDimensions()
	grid.SetRect(0, 0, termWidth, termHeight)

	grid.Set(
		ui.NewRow(1.0/2,
			ui.NewCol(1.0/2, p),
			ui.NewCol(1.0/2, l),
		),
		ui.NewRow(1.0/2,
			ui.NewCol(1.0/4, g),
			ui.NewCol(1.0/4,
				ui.NewRow(.9/3, slg),
				ui.NewRow(.9/3, lc),
				ui.NewRow(1.2/3, bc),
			),
			ui.NewCol(1.0/2, lc2),
		),
	)

	// setup function to call whenever
	// there is new data
	draw := func(count int, paragraph string) {
		g.Percent = count % 101
		l.Rows = listData[count%9:]
		slg.Sparklines[0].Data = sparklineData[:30+count%50]
		slg.Sparklines[1].Data = sparklineData[:35+count%50]
		lc.Data[0] = sinData[count/2%220:]
		lc2.Data[0] = sinData[2*count%220:]
		bc.Data = barchartData[count/2%10:]
		if paragraph != "" {
			p.Text = paragraph
		}
		ui.Render(grid)
	}
	// show the initial ui to the user
	ui.Render(grid)

	return &GUI{
		grid:            grid,
		updateParagraph: updateParagraph,
		draw:            draw,
	}, nil
}
