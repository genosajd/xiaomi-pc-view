package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP server for AI assistants: `xiaomi-cam mcp` (stdio). It controls the camera through
// the local HTTP API of the running app and starts the app hidden in the tray if needed.
//
// Claude Code:  claude mcp add xiaomi-cam -- ~/.local/bin/xiaomi-cam mcp

func runMCP() error {
	if err := ensureAppRunning(); err != nil {
		return err
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "xiaomi-cam", Version: version}, &mcp.ServerOptions{
		Instructions: "Controls Xiaomi Mi Home cameras (e.g. Outdoor Camera CW700S): live snapshots, " +
			"pan/tilt, camera zoom, spotlight, siren, saving snapshots and MP4 recordings to disk. Use get_snapshot to see what the camera sees; " +
			"after moving or zooming wait a moment (the camera needs ~1-2 s) and take a new snapshot.",
	})

	type cameraArg struct {
		Camera string `json:"camera,omitempty" jsonschema:"camera name from list_cameras; default is the first camera"`
	}

	mcp.AddTool(server, &mcp.Tool{Name: "list_cameras", Description: "List cameras configured in Xiaomi Cam."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return apiText("GET", "cameras", nil)
		})

	type snapshotArgs struct {
		cameraArg
		Width int `json:"width,omitempty" jsonschema:"image width in pixels (keeps aspect ratio), default 1280, max 2560"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "get_snapshot", Description: "Take a live JPEG snapshot from the camera (full sensor is 2560x1440). The camera prints date and time in the top left corner."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in snapshotArgs) (*mcp.CallToolResult, any, error) {
			w := in.Width
			if w <= 0 {
				w = 1280
			}
			b, err := camAPI("GET", "snapshot", url.Values{"camera": {in.Camera}, "width": {strconv.Itoa(min(w, 2560))}})
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: b, MIMEType: "image/jpeg"}}}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "get_camera_state", Description: "Get camera state: zoom, spotlight on/off and brightness, siren, night vision mode."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cameraArg) (*mcp.CallToolResult, any, error) {
			return apiText("GET", "state", url.Values{"camera": {in.Camera}})
		})

	type moveArgs struct {
		cameraArg
		Direction string `json:"direction" jsonschema:"left, right, up or down"`
		Steps     int    `json:"steps,omitempty" jsonschema:"how long to move, in steps of 0.1 s (10 = 1 s); default 10, max 100"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "move_camera", Description: "Pan/tilt the camera."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in moveArgs) (*mcp.CallToolResult, any, error) {
			if in.Steps <= 0 {
				in.Steps = 10
			}
			return apiText("POST", "move", url.Values{"camera": {in.Camera}, "direction": {in.Direction}, "steps": {strconv.Itoa(in.Steps)}})
		})

	type zoomArgs struct {
		cameraArg
		Zoom float64 `json:"zoom" jsonschema:"camera zoom from 1.0 (wide) to 9.0"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "set_zoom", Description: "Set the camera zoom (done by the camera, sharp details). " +
		"A dual-lens camera (get_camera_state has tele_zoom) switches to its telephoto lens at tele_zoom: the sharpest far detail."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in zoomArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "zoom", url.Values{"camera": {in.Camera}, "value": {strconv.FormatFloat(in.Zoom, 'f', 1, 64)}})
		})

	type aimArgs struct {
		cameraArg
		X    float64 `json:"x" jsonschema:"the point across the picture as a fraction of its width from the centre: -0.5 left edge, 0 centre, 0.5 right edge"`
		Y    float64 `json:"y" jsonschema:"the point down the picture as a fraction of its height from the centre: -0.5 top, 0.5 bottom"`
		Lens string  `json:"lens,omitempty" jsonschema:"tele (look closer), wide, or empty: the lens the camera is on"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "aim_camera", Description: "Turn a CW700S camera so that a point of its current picture comes to the centre " +
		"(take a snapshot first and pick the point in it); with lens=tele it also switches to the telephoto lens to look closer. " +
		"Manual control: the camera goes back to its watch view after a few idle minutes (get_camera_control)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in aimArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "aim", url.Values{"camera": {in.Camera}, "x": {strconv.FormatFloat(in.X, 'f', 3, 64)},
				"y": {strconv.FormatFloat(in.Y, 'f', 3, 64)}, "lens": {in.Lens}})
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_camera_control", Description: "Whether a camera is away from its watch view after manual control, " +
		"and in how many seconds it goes back by itself."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cameraArg) (*mcp.CallToolResult, any, error) {
			return apiText("GET", "control", url.Values{"camera": {in.Camera}})
		})

	type lensArgs struct {
		cameraArg
		Lens string `json:"lens" jsonschema:"wide or tele"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "set_lens", Description: "Switch a dual-lens camera (CW700S) between its wide-angle and telephoto lens. " +
		"The camera sends one lens at a time: everyone watching the camera sees the lens chosen."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in lensArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "lens", url.Values{"camera": {in.Camera}, "name": {in.Lens}})
		})

	type spotlightArgs struct {
		cameraArg
		On         *bool `json:"on,omitempty" jsonschema:"turn the spotlight on or off"`
		Brightness *int  `json:"brightness,omitempty" jsonschema:"spotlight brightness 0-100"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "set_spotlight", Description: "Turn the camera spotlight (white light) on/off and set its brightness."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in spotlightArgs) (*mcp.CallToolResult, any, error) {
			q := url.Values{"camera": {in.Camera}}
			if in.On != nil {
				q.Set("on", strconv.FormatBool(*in.On))
			}
			if in.Brightness != nil {
				q.Set("brightness", strconv.Itoa(*in.Brightness))
			}
			return apiText("POST", "spotlight", q)
		})

	type nightArgs struct {
		cameraArg
		Mode string `json:"mode" jsonschema:"auto (infrared when dark), auto-color (full color with the spotlight when dark), smart-color (full color when a person is detected), on (always infrared), color (always full color), off"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "set_night_mode", Description: "Set the camera night vision mode: infrared (black and white) or full color with the spotlight."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nightArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "night-mode", url.Values{"camera": {in.Camera}, "mode": {in.Mode}})
		})

	type sirenArgs struct {
		cameraArg
		On bool `json:"on" jsonschema:"true - start the siren, false - stop it"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "set_siren", Description: "Start or stop the camera alarm siren. It is loud - use only when the user asks for it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in sirenArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "siren", url.Values{"camera": {in.Camera}, "on": {strconv.FormatBool(in.On)}})
		})

	mcp.AddTool(server, &mcp.Tool{Name: "save_snapshot", Description: "Save a full resolution (2560x1440) JPEG snapshot to the snapshots folder on disk. Returns the file path."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cameraArg) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "snapshot/save", url.Values{"camera": {in.Camera}})
		})

	type recordArgs struct {
		cameraArg
		Duration int      `json:"duration_seconds,omitempty" jsonschema:"stop automatically after this many seconds; 0 or empty - record until stop_recording"`
		Cameras  []string `json:"cameras,omitempty" jsonschema:"cameras for the each/grid modes; default: cameras checked in the app settings"`
		Mode     string   `json:"mode,omitempty" jsonschema:"selected (the camera argument), each (every camera to its own file) or grid (all cameras in one file); default from the app settings"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "start_recording", Description: "Start recording the camera to an MP4 file (H.265 video as from the camera, AAC audio) in the recordings folder. Returns the file path."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in recordArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "record/start", url.Values{"camera": {in.Camera}, "duration": {strconv.Itoa(in.Duration)}, "mode": {in.Mode}, "cameras": {strings.Join(in.Cameras, ",")}})
		})

	mcp.AddTool(server, &mcp.Tool{Name: "stop_recording", Description: "Stop all recordings and finish the MP4 files. Returns the file paths and durations."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cameraArg) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "record/stop", url.Values{"camera": {in.Camera}})
		})

	mcp.AddTool(server, &mcp.Tool{Name: "recording_status", Description: "Is the camera being recorded; file path and elapsed seconds."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cameraArg) (*mcp.CallToolResult, any, error) {
			return apiText("GET", "record", url.Values{"camera": {in.Camera}})
		})

	type eventsArgs struct {
		cameraArg
		Hours int `json:"hours,omitempty" jsonschema:"the last N hours (default 24, up to 744)"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "list_events", Description: "Events detected by the camera (motion, person, car, a person near a car, entering/leaving the virtual fence) with their times, from the SD card recordings. Use get_snapshot to see the camera now."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in eventsArgs) (*mcp.CallToolResult, any, error) {
			return apiText("GET", "events", url.Values{"camera": {in.Camera}, "hours": {strconv.Itoa(in.Hours)}})
		})

	mcp.AddTool(server, &mcp.Tool{Name: "list_points", Description: "List camera points 1-3 (saved positions, e.g. 1 = car, 2 = driveway) with their names."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cameraArg) (*mcp.CallToolResult, any, error) {
			return apiText("GET", "points", url.Values{"camera": {in.Camera}})
		})

	type pointArgs struct {
		cameraArg
		Point string `json:"point" jsonschema:"point number 1-3 or its name from list_points"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "goto_point", Description: "Turn the camera to a saved point and wait until it stops; then take a snapshot to see it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in pointArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "points/goto", url.Values{"camera": {in.Camera}, "point": {in.Point}})
		})

	type savePointArgs struct {
		cameraArg
		Point int    `json:"point" jsonschema:"point number 1-3"`
		Name  string `json:"name,omitempty" jsonschema:"name of the point, e.g. car, driveway"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "save_point", Description: "Save the current camera position (direction and zoom) as point 1-3. Overwrites the point."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in savePointArgs) (*mcp.CallToolResult, any, error) {
			return apiText("POST", "points/save", url.Values{"camera": {in.Camera}, "point": {strconv.Itoa(in.Point)}, "name": {in.Name}})
		})

	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func camAPI(method, path string, q url.Values) ([]byte, error) {
	if q != nil && q.Get("camera") == "" {
		q.Del("camera")
	}
	u := "http://" + apiAddr + "/api/xiaomi-cam/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{Timeout: 90 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", b)
	}
	return b, nil
}

func apiText(method, path string, q url.Values) (*mcp.CallToolResult, any, error) {
	b, err := camAPI(method, path, q)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

// ensureAppRunning starts Xiaomi Cam hidden in the tray if it isn't running.
func ensureAppRunning() error {
	ping := func() bool {
		b, err := camAPI("GET", "cameras", nil)
		return err == nil && json.Valid(b)
	}
	if ping() {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--tray")
	detach(cmd) // keep running after the MCP client exits
	if err = cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	for i := 0; i < 40; i++ {
		time.Sleep(500 * time.Millisecond)
		if ping() {
			return nil
		}
	}
	return errors.New("xiaomi-cam app didn't start")
}
