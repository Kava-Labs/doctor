package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"

	"github.com/kava-labs/rosetta-kava/kava"
)

var (
	ctx = context.Background()

	interactiveModeFlag = flag.Bool("interactive", false, "controls whether an interactive terminal UI is displayed")
	debugModeFlag       = flag.Bool("debug", false, "controls whether debug logging is enabled")
)

func main() {
	// parse flags
	flag.Parse()

	interactiveMode := *interactiveModeFlag
	debugMode := *debugModeFlag

	// setup default logger
	var logger *log.Logger

	if debugMode {
		logger = log.New(os.Stdout, "doctor ", log.LstdFlags|log.Lshortfile)
		logger.Print("debug logging enabled")
	} else {
		// log to dev null
		logger = log.New(ioutil.Discard, "doctor ", log.LstdFlags|log.Lshortfile)
	}

	// TODO: setup and parse config using viper
	kavaNodeRPCURL := os.Getenv("KAVA_RPC_URL")

	// setup kava client
	http, err := kava.NewHTTPClient(kavaNodeRPCURL)

	if err != nil {
		panic(fmt.Errorf("%w: could not initialize http client", err))
	}

	accountBalanceFactory := kava.NewRPCBalanceFactory(http)

	client, err := kava.NewClient(http, accountBalanceFactory)

	if err != nil {
		panic(fmt.Errorf("%w: could not initialize kava client", err))
	}

	// setup channel for new monitoring data
	observedBlockHeights := make(chan int64, 1)
	logMessages := make(chan string, 1024)

	// run monitoring routines
	go func() {
		for {
			_, _, _, status, _, err := client.Status(ctx)

			if err != nil {
				// log error, but don't block the monitoring
				// routine if the logMessage channel is full
				go func() {
					logMessages <- fmt.Sprintf("error %s getting node status", err)
				}()

				// give the endpoint time to recover
				time.Sleep(5 * time.Second)

				// repoll
				continue
			}

			observedBlockHeights <- *status.CurrentIndex

			time.Sleep(5 * time.Second)
		}
	}()

	// setup event handlers for interactive mode
	if interactiveMode {
		if err := ui.Init(); err != nil {
			panic(fmt.Errorf("failed to initialize termui: %v", err))
		}

		defer ui.Close()

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

		lc2 := widgets.NewPlot()
		lc2.Title = "braille-mode Line Chart"
		lc2.Data = make([][]float64, 1)
		lc2.Data[0] = sinData
		lc2.SetRect(50, 15, 75, 25)
		lc2.AxesColor = ui.ColorWhite
		lc2.LineColors[0] = ui.ColorYellow

		p2 := widgets.NewParagraph()
		p2.Text = "Hey!\nI am a borderless block!"
		p2.Border = false
		p2.SetRect(50, 10, 75, 10)
		p2.TextStyle.Fg = ui.ColorMagenta

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
		draw := func(count int) {
			g.Percent = count % 101
			l.Rows = listData[count%9:]
			slg.Sparklines[0].Data = sparklineData[:30+count%50]
			slg.Sparklines[1].Data = sparklineData[:35+count%50]
			lc.Data[0] = sinData[count/2%220:]
			lc2.Data[0] = sinData[2*count%220:]
			bc.Data = barchartData[count/2%10:]

			ui.Render(grid)
		}
		// show the initial ui to the user
		ui.Render(grid)

		tickerCount := 1
		// create channel to subscribe to
		// user input
		uiEvents := ui.PollEvents()
		// create channel to subscribe to
		//
		ticker := time.NewTicker(time.Second).C

		for {
			select {
			// events triggered by user input
			// or action such as keyboard strokes
			// mouse movements or window changes
			case e := <-uiEvents:
				switch e.ID {
				case "q", "<C-c>":
					return
				case "<Resize>":
					payload := e.Payload.(ui.Resize)
					grid.SetRect(0, 0, payload.Width, payload.Height)
					ui.Clear()
					ui.Render(grid)
				}
			// events triggered by new data
			case newBlockHeight := <-observedBlockHeights:
				p.Text = fmt.Sprintf(`
				Latest Block Height %d
				Other Metric %d
				`, newBlockHeight, 6)
				draw(tickerCount)
			case logMessage := <-logMessages:
				// TODO: seperate channels
				// for debug only log messages?
				// if !debugMode {
				// 	continue
				// }
				p.Text = logMessage
				draw(tickerCount)
				time.Sleep(2 * time.Second)
			// events triggered on a regular time based interval
			case <-ticker:
				updateParagraph(tickerCount)
				draw(tickerCount)
				tickerCount++
			}
		}
	}

	// event handlers for non-interactive mode
	// loop over events
	for {
		select {
		case newBlockHeight := <-observedBlockHeights:
			// TODO: log to configured backends (stdout, file and or cloudwatch)
			// for now log new monitoring data to stdout by default
			fmt.Printf("%s is synched up to block %d @ %d", kavaNodeRPCURL, newBlockHeight, time.Now().Unix())
			// TODO: check to see if we should log this to a file
			// TODO: check to see if we should this to cloudwatch
		case logMessage := <-logMessages:
			logger.Print(logMessage)
		}
	}
}
