package main

import "github.com/AlexxIT/go2rtc/internal/xccore"

// The camera logic is in internal/xccore (shared with the Android app); short names here.

type (
	miotProp   = xccore.MiotProp
	preset     = xccore.Preset
	camCaps    = xccore.Caps
	ipCamera   = xccore.IPCamera
	imouDevice = xccore.ImouDevice
	nightMode  = xccore.NightMode
	mover      = xccore.Mover
)

const (
	hiddenPrefix = xccore.HiddenPrefix
	motorLeft    = xccore.MotorLeft
	motorRight   = xccore.MotorRight
	motorUp      = xccore.MotorUp
	motorDown    = xccore.MotorDown
	presetSlots  = xccore.PresetSlots
	maxPresets   = xccore.MaxPresets
)

var (
	errNotXiaomi = xccore.ErrNotXiaomi
	allCaps      = xccore.AllCaps
	nightModes   = xccore.NightModes

	propLight         = xccore.PropLight
	propBrightness    = xccore.PropBrightness
	propSiren         = xccore.PropSiren
	propZoom          = xccore.PropZoom
	propSDStatus      = xccore.PropSDStatus
	propNightShot     = xccore.PropNightShot
	propWDR           = xccore.PropWDR
	propLowLightColor = xccore.PropLowLightColor
	propImageRollover = xccore.PropImageRollover
	propMotorLocation = xccore.PropMotorLocation
	propPresets       = xccore.PropPresets
	propPresetActive  = xccore.PropPresetActive

	apiCall          = xccore.APICall
	cameraNames      = xccore.CameraNames
	cameraSource     = xccore.CameraSource
	sourceParam      = xccore.SourceParam
	setSourceParams  = xccore.SetSourceParams
	setPlayback      = xccore.SetPlayback
	addCamera        = xccore.AddCamera
	removeCamera     = xccore.RemoveCamera
	rtspURL          = xccore.RTSPURL
	motor            = xccore.Motor
	sdFiles          = xccore.SDFiles
	sdEvents         = xccore.SDEvents
	cameraEvents     = xccore.CameraEvents
	getDetection     = xccore.GetDetection
	cameraZoom       = xccore.CameraZoom
	teleZoom         = xccore.TeleZoom
	currentZoom      = xccore.CurrentZoom
	setZoomState     = xccore.SetZoomState
	cameraDid        = xccore.CameraDid
	getProps         = xccore.GetProps
	setProp          = xccore.SetProp
	cameraCaps       = xccore.CameraCaps
	nightModeByValue = xccore.NightModeByValue
	nightModeByID    = xccore.NightModeByID
	cameraNightModes = xccore.CameraNightModes
	getNightMode     = xccore.GetNightMode
	setNightMode     = xccore.SetNightMode
	startONVIFEvents = xccore.StartONVIFEvents
	hasImou          = xccore.HasImou
	imouDevices      = xccore.ImouDevices
	imouDataCenters  = xccore.ImouDataCenters

	getPresets        = xccore.GetPresets
	putPresets        = xccore.PutPresets
	deletePreset      = xccore.DeletePreset
	gotoPreset        = xccore.GotoPreset
	gotoPresetAndWait = xccore.GotoPresetAndWait
	pointReached      = xccore.PointReached
	savePresetSlot    = xccore.SavePresetSlot
	renamePresetSlot  = xccore.RenamePresetSlot
	presetSlotsOf     = xccore.PresetSlotsOf
	findPoint         = xccore.FindPoint

	loadIPCameras  = xccore.LoadIPCameras
	ipCam          = xccore.IPCam
	saveIPCamera   = xccore.SaveIPCamera
	addIPCamera    = xccore.AddIPCamera
	updateIPCamera = xccore.UpdateIPCamera
	setIPQuality   = xccore.SetIPQuality
	cameraExists   = xccore.CameraExists
	streamSource   = xccore.StreamSource

	newMover = xccore.NewMover
)
