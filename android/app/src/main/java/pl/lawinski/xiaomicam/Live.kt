package pl.lawinski.xiaomicam

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import android.content.Context
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.VolumeOff
import androidx.compose.material.icons.automirrored.rounded.VolumeUp
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.FlashlightOn
import androidx.compose.material.icons.rounded.GridView
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.NightsStay
import androidx.compose.material.icons.rounded.Remove
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.positionChange
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.abs

/** how long the controls stay on the picture after the last touch / key */
private const val CONTROLS_IDLE_MS = 5000L

// LiveScreen - the camera picture with its controls. Layouts:
//  - normal (phone, TV): the picture fills the screen, controls lie on it and hide when idle;
//  - wide (car radio 2560x720, 32:9): the 16:9 picture on the left, big touch controls on the
//    right, always shown.
// Remote (TV): the picture has the focus; arrows move the camera while held, OK shows the
// controls, Back hides them; keys 1-3 go to the camera points.
@Composable
fun LiveScreen(
    camera: Camera,
    tv: Boolean,
    onPickCamera: () -> Unit,
    onGrid: (() -> Unit)?, // null: one camera only
    onMenu: () -> Unit,
    onError: (String) -> Unit,
) {
    val s = strings()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var status by remember { mutableStateOf(VideoStatus.CONNECTING) }
    var statusText by remember { mutableStateOf<String?>(null) }
    var caps by remember(camera.name) { mutableStateOf(Caps.NONE) }
    var qualities by remember(camera.name) { mutableStateOf(Qualities(emptyList(), emptyList(), 0)) }
    var ready by remember(camera.name) { mutableStateOf(false) } // the quality is checked: connect
    var generation by remember { mutableIntStateOf(0) } // a new video connection
    var points by remember(camera.name) { mutableStateOf<List<Point>>(emptyList()) }
    var state by remember(camera.name) { mutableStateOf(CamState(null, null)) }
    var zoom by remember(camera.name) { mutableStateOf(1.0) }
    val prefs = remember { context.getSharedPreferences(PREFS, Context.MODE_PRIVATE) }
    var muted by remember { mutableStateOf(!prefs.getBoolean(PREF_SOUND, false)) }
    var hasAudio by remember(camera.name) { mutableStateOf(false) }
    var lastInput by remember { mutableLongStateOf(System.currentTimeMillis()) }
    var shown by remember { mutableStateOf(true) }
    val videoFocus = remember { FocusRequester() }
    val controlsFocus = remember { FocusRequester() }

    fun touch() {
        lastInput = System.currentTimeMillis()
        shown = true
    }

    fun act(block: suspend () -> Unit) {
        touch()
        scope.launch {
            try {
                block()
            } catch (e: Exception) {
                onError(e.message ?: e.toString())
            }
        }
    }

    LaunchedEffect(camera.name) {
        qualities = Core.qualities(camera.name)
        ready = true
        zoom = Core.currentZoom(camera.name)
        caps = Core.caps(camera.name)
        if (caps.points) points = runCatching { Core.points(camera.name) }.getOrDefault(emptyList())
        if (caps.light || caps.nightMode) {
            state = runCatching { Core.state(camera.name) }.getOrDefault(state)
        }
    }
    LaunchedEffect(lastInput, status) {
        delay(CONTROLS_IDLE_MS)
        if (status == VideoStatus.PLAYING) shown = false
    }
    LaunchedEffect(Unit) { runCatching { videoFocus.requestFocus() } }

    val zoomTo = { z: Double ->
        val nz = z.coerceIn(1.0, 9.0)
        zoom = nz
        act { Core.zoom(camera.name, nz) }
    }
    val goto = { p: Point ->
        if (p.saved) act {
            Core.gotoPoint(camera.name, p.slot)
            zoom = Core.currentZoom(camera.name)
        }
    }

    BoxWithConstraints(
        Modifier
            .fillMaxSize()
            .background(Palette.bg)
    ) {
        // wide: room for a control panel next to the 16:9 picture (a car radio 2560x720; not a
        // phone in landscape: 20:9 leaves a strip too narrow for the controls)
        val wide = maxWidth - maxHeight * 16f / 9f >= 360.dp
        val portrait = maxHeight > maxWidth
        LaunchedEffect(wide, portrait) { touch() } // turned: show the controls
        val size = if (wide || tv) SizeLarge else SizeNormal

        val video = @Composable { mod: Modifier ->
            Box(
                mod
                    .background(Palette.bg)
                    .focusRequester(videoFocus)
                    .onFocusChanged { }
                    .onKeyEvent { e ->
                        val op = when (e.key) {
                            Key.DirectionLeft -> Core.MOTOR_LEFT
                            Key.DirectionRight -> Core.MOTOR_RIGHT
                            Key.DirectionUp -> Core.MOTOR_UP
                            Key.DirectionDown -> Core.MOTOR_DOWN
                            else -> 0L
                        }
                        when {
                            op != 0L && caps.ptz -> {
                                if (e.type == KeyEventType.KeyDown) Core.motorPress(camera.name, op) else Core.motorRelease()
                                true
                            }
                            (e.key == Key.DirectionCenter || e.key == Key.Enter) && e.type == KeyEventType.KeyUp -> {
                                touch()
                                scope.launch { delay(50); runCatching { controlsFocus.requestFocus() } }
                                true
                            }
                            else -> false
                        }
                    }
                    .focusable()
                    .pointerInput(caps.ptz) {
                        // tap: show/hide the controls; drag: move the camera while the finger moves
                        awaitEachGesture {
                            val down = awaitFirstDown()
                            var moving = 0L
                            var dragged = false
                            var total = androidx.compose.ui.geometry.Offset.Zero
                            while (true) {
                                val ev = awaitPointerEvent()
                                val ch = ev.changes.firstOrNull { it.id == down.id } ?: break
                                if (!ch.pressed) break
                                total += ch.positionChange()
                                if (caps.ptz && (abs(total.x) > 40 || abs(total.y) > 40)) {
                                    dragged = true
                                    val op = if (abs(total.x) > abs(total.y)) {
                                        if (total.x < 0) Core.MOTOR_RIGHT else Core.MOTOR_LEFT // drag the picture
                                    } else {
                                        if (total.y < 0) Core.MOTOR_DOWN else Core.MOTOR_UP
                                    }
                                    if (op != moving) {
                                        moving = op
                                        Core.motorPress(camera.name, op)
                                    }
                                }
                            }
                            if (moving != 0L) Core.motorRelease()
                            if (!dragged) {
                                if (shown && !wide && !portrait) shown = false else touch()
                            } else touch()
                        }
                    },
                contentAlignment = Alignment.Center,
            ) {
                if (ready) CameraPlayer(
                    url = Core.rtspUrlAudio(camera.name),
                    videoUrl = Core.rtspUrl(camera.name),
                    key = camera.name to generation,
                    modifier = Modifier.fillMaxSize(),
                    muted = muted,
                    onAudio = { hasAudio = it },
                ) { st, msg ->
                    status = st
                    statusText = msg
                }
                if (status == VideoStatus.CONNECTING) {
                    CircularProgressIndicator(color = Palette.accent, strokeWidth = 3.dp, modifier = Modifier.size(48.dp))
                }
            }
        }

        val pickQuality = { i: Int ->
            if (i != qualities.selected) act {
                Core.setQuality(camera.name, i)
                qualities = qualities.copy(selected = i)
                generation++ // reconnect: the camera session is redialed with the new quality
            }
        }

        val cameraChip = @Composable {
            Pill {
                XcButton(
                    onClick = onPickCamera, text = camera.name, label = s.cameras, icon = Icons.Rounded.ExpandMore,
                    size = size, modifier = Modifier.focusRequester(controlsFocus),
                )
            }
        }
        val qualityPicker = @Composable {
            if (qualities.names.size > 1) {
                Pill {
                    Row {
                        qualities.names.forEachIndexed { i, n ->
                            XcButton(onClick = { pickQuality(i) }, text = n, active = i == qualities.selected, size = size)
                        }
                    }
                }
            }
        }
        val menuButton = @Composable {
            Pill {
                Row {
                    if (hasAudio) XcButton(
                        onClick = {
                            touch()
                            muted = !muted
                            prefs.edit().putBoolean(PREF_SOUND, !muted).apply()
                        },
                        icon = if (muted) Icons.AutoMirrored.Rounded.VolumeOff else Icons.AutoMirrored.Rounded.VolumeUp,
                        label = if (muted) s.soundOn else s.soundOff, size = size,
                    )
                    if (onGrid != null) XcButton(onClick = onGrid, icon = Icons.Rounded.GridView, label = s.grid, size = size)
                    XcButton(onClick = onMenu, icon = Icons.Rounded.MoreVert, label = s.menu, size = size)
                }
            }
        }
        val topBar = @Composable { mod: Modifier ->
            val row = Arrangement.spacedBy(8.dp)
            if (portrait) {
                // narrow: the camera and the menu, below the status and the quality
                Column(mod, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = row) {
                        Box(Modifier.weight(1f)) { cameraChip() }
                        menuButton()
                    }
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = row) {
                        StatusChip(status, statusText, size)
                        Spacer(Modifier.weight(1f))
                        qualityPicker()
                    }
                }
            } else {
                Row(mod, verticalAlignment = Alignment.CenterVertically, horizontalArrangement = row) {
                    cameraChip()
                    StatusChip(status, statusText, size)
                    Spacer(Modifier.weight(1f))
                    qualityPicker()
                    menuButton()
                }
            }
        }

        val zoomControl = @Composable { vertical: Boolean ->
            if (caps.zoom) {
                Pill {
                    val label = @Composable {
                        Text(
                            "%.0f×".format(zoom), Modifier.width(size.button),
                            style = MaterialTheme.typography.labelLarge, textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                        )
                    }
                    val plus = @Composable { XcButton(onClick = { zoomTo(zoom + 1) }, icon = Icons.Rounded.Add, label = s.zoomIn, size = size, enabled = zoom < 9.0) }
                    val minus = @Composable { XcButton(onClick = { zoomTo(zoom - 1) }, icon = Icons.Rounded.Remove, label = s.zoomOut, size = size, enabled = zoom > 1.0) }
                    if (vertical) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) { plus(); label(); minus() }
                    } else {
                        Row(verticalAlignment = Alignment.CenterVertically) { minus(); label(); plus() }
                    }
                }
            }
        }

        val pointsControl = @Composable {
            if (caps.points && points.any { it.saved }) {
                Pill {
                    Row {
                        points.forEach { p ->
                            XcButton(
                                onClick = { goto(p) }, enabled = p.saved, size = size,
                                text = if (p.name.isNotEmpty() && p.name != p.slot.toString()) "${p.slot} ${p.name}" else "${p.slot}",
                                label = s.point(p.slot),
                            )
                        }
                    }
                }
            }
        }

        val lightControls = @Composable {
            if (caps.light || caps.nightMode) {
                Pill {
                    Row {
                        if (caps.light) XcButton(
                            onClick = {
                                val on = state.light != true
                                state = state.copy(light = on)
                                act { Core.setLight(camera.name, on) }
                            },
                            icon = Icons.Rounded.FlashlightOn, label = s.spotlight, active = state.light == true, size = size,
                        )
                        if (caps.nightMode) XcButton(
                            onClick = {
                                // the next mode of the list (the desktop order)
                                val modes = Core.nightModes
                                val i = modes.indexOfFirst { it.value == state.night }
                                val next = modes[(i + 1).mod(modes.size)]
                                state = state.copy(night = next.value)
                                act { Core.setNightMode(camera.name, next.value) }
                            },
                            icon = Icons.Rounded.NightsStay, text = s.nightMode(state.night), label = s.nightModeLabel, size = size,
                        )
                    }
                }
            }
        }

        val keys = Modifier.onPreviewKeyEvent { e ->
            touch()
            if (e.type != KeyEventType.KeyUp) return@onPreviewKeyEvent false
            when (e.key) {
                Key.One, Key.NumPad1 -> { points.getOrNull(0)?.let(goto); true }
                Key.Two, Key.NumPad2 -> { points.getOrNull(1)?.let(goto); true }
                Key.Three, Key.NumPad3 -> { points.getOrNull(2)?.let(goto); true }
                Key.ChannelUp, Key.ZoomIn -> { if (caps.zoom) zoomTo(zoom + 1); true }
                Key.ChannelDown, Key.ZoomOut -> { if (caps.zoom) zoomTo(zoom - 1); true }
                Key.VolumeMute, Key.M -> {
                    if (hasAudio) {
                        muted = !muted
                        prefs.edit().putBoolean(PREF_SOUND, !muted).apply()
                    }
                    true
                }
                else -> false
            }
        }

        if (wide) {
            // car radio: the picture left, the controls right (always shown, big)
            CompositionLocalProvider(LocalPillColor provides Palette.surface) { Row(keys.fillMaxSize().safeDrawingPadding()) {
                video(Modifier.fillMaxHeight().aspectRatio(16f / 9f, matchHeightConstraintsFirst = true))
                Column(
                    Modifier
                        .weight(1f)
                        .fillMaxHeight()
                        .padding(16.dp),
                    verticalArrangement = Arrangement.SpaceBetween,
                ) {
                    topBar(Modifier.fillMaxWidth())
                    Row(
                        Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceEvenly,
                    ) {
                        if (caps.ptz) PtzPad(camera.name, size)
                        Column(verticalArrangement = Arrangement.spacedBy(16.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                            zoomControl(false)
                            pointsControl()
                            lightControls()
                        }
                    }
                }
            } }
        } else if (portrait) {
            // phone held upright: the 16:9 picture on top, the controls below (always shown)
            CompositionLocalProvider(LocalPillColor provides Palette.surface) { Column(keys.fillMaxSize().safeDrawingPadding()) {
                topBar(Modifier.fillMaxWidth().padding(12.dp))
                video(Modifier.fillMaxWidth().aspectRatio(16f / 9f))
                Column(
                    Modifier.fillMaxWidth().weight(1f).padding(16.dp),
                    verticalArrangement = Arrangement.SpaceEvenly,
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    if (caps.ptz) PtzPad(camera.name, SizeLarge)
                    pointsControl()
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                        lightControls()
                        zoomControl(false)
                    }
                }
            } }
        } else {
            Box(keys.fillMaxSize()) {
                video(Modifier.fillMaxSize())
                AnimatedVisibility(shown, enter = fadeIn(), exit = fadeOut(), modifier = Modifier.fillMaxSize()) {
                    Box(Modifier.fillMaxSize().safeDrawingPadding().padding(12.dp)) {
                        topBar(Modifier.fillMaxWidth().align(Alignment.TopCenter))
                        // like a camera app: pan/tilt bottom left, zoom upright at the right edge,
                        // points and lights in one row at the bottom right
                        if (caps.ptz) PtzPad(camera.name, size, Modifier.align(Alignment.BottomStart))
                        Box(Modifier.align(Alignment.CenterEnd)) { zoomControl(true) }
                        Row(
                            Modifier.align(Alignment.BottomEnd),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            pointsControl()
                            lightControls()
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun StatusChip(status: VideoStatus, text: String?, size: ControlSize) {
    val s = strings()
    Row(
        Modifier
            .clip(RoundedCornerShape(50))
            .background(LocalPillColor.current)
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val (color, label) = when (status) {
            VideoStatus.PLAYING -> Palette.live to s.live
            VideoStatus.CONNECTING -> Palette.accent to s.connecting
            VideoStatus.ERROR -> Palette.rec to s.noVideo
        }
        Box(Modifier.size(8.dp).clip(CircleShape).background(color))
        Spacer(Modifier.width(8.dp))
        Text(
            label, style = if (size == SizeLarge) MaterialTheme.typography.labelLarge else MaterialTheme.typography.labelMedium,
            maxLines = 1, overflow = TextOverflow.Ellipsis,
        )
    }
}
