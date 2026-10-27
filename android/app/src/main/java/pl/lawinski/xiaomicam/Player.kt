package pl.lawinski.xiaomicam

import android.content.Context
import android.view.LayoutInflater
import androidx.annotation.OptIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.Tracks
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.rtsp.RtspMediaSource
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import android.os.Handler
import android.os.Looper

enum class VideoStatus { CONNECTING, PLAYING, ERROR }

// CameraPlayer plays the local RTSP stream of go2rtc (the camera session is opened by go2rtc
// when the player connects and closed when it disconnects). Hardware decoding of H.265 and
// H.264 by MediaCodec; small buffers: live video, not smooth playback. On an error it
// reconnects after 3 s. key: a change (camera, quality) starts a new connection.
// Sound: url with the audio track and videoUrl without it; when the audio track can't be
// read or decoded the player falls back to videoUrl. muted only sets the volume (no
// reconnect); onAudio reports whether the stream has a playable audio track.
@OptIn(UnstableApi::class)
@Composable
fun CameraPlayer(
    url: String,
    key: Any,
    modifier: Modifier = Modifier,
    videoUrl: String? = null,
    muted: Boolean = true,
    onAudio: (Boolean) -> Unit = {},
    onStatus: (VideoStatus, String?) -> Unit,
) {
    val context = LocalContext.current
    val player = remember(key) { newPlayer(context) }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(player, muted) { player.volume = if (muted) 0f else 1f }
    DisposableEffect(player) {
        val handler = Handler(Looper.getMainLooper())
        var current = url
        val play = {
            onStatus(VideoStatus.CONNECTING, null)
            val source = RtspMediaSource.Factory()
                .setForceUseRtpTcp(true) // go2rtc serves RTP over the RTSP connection
                .setTimeoutMs(30_000)
                .createMediaSource(MediaItem.fromUri(current))
            player.setMediaSource(source)
            player.prepare()
            player.playWhenReady = true
        }
        val listener = object : Player.Listener {
            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_READY) onStatus(VideoStatus.PLAYING, null)
            }

            override fun onTracksChanged(tracks: Tracks) {
                onAudio(tracks.isTypeSupported(C.TRACK_TYPE_AUDIO))
            }

            override fun onPlayerError(error: PlaybackException) {
                // the audio track can't be parsed, decoded or played: video only
                if (videoUrl != null && current != videoUrl && error.errorCode >= PlaybackException.ERROR_CODE_PARSING_CONTAINER_MALFORMED) {
                    current = videoUrl
                    onAudio(false)
                }
                onStatus(VideoStatus.ERROR, error.cause?.message ?: error.errorCodeName)
                handler.postDelayed({ if (lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) play() }, 3000)
            }
        }
        player.addListener(listener)
        // in the background (screen off, another app) the stream stops: no battery use, and the
        // camera session is free (the camera takes one at a time: the PC app can connect)
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_START -> play()
                Lifecycle.Event.ON_STOP -> {
                    handler.removeCallbacksAndMessages(null)
                    player.stop()
                }
                else -> {}
            }
        }
        lifecycle.addObserver(observer) // calls ON_START right away when started
        onDispose {
            lifecycle.removeObserver(observer)
            handler.removeCallbacksAndMessages(null)
            player.removeListener(listener)
            player.release()
        }
    }
    AndroidView(
        modifier = modifier,
        factory = { ctx ->
            (LayoutInflater.from(ctx).inflate(R.layout.player_view, null) as PlayerView).apply {
                useController = false
                resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
                isFocusable = false
            }
        },
        update = { it.player = player },
    )
}

@OptIn(UnstableApi::class)
private fun newPlayer(context: Context): ExoPlayer {
    val load = DefaultLoadControl.Builder()
        .setBufferDurationsMs(500, 2000, 200, 200)
        .setPrioritizeTimeOverSizeThresholds(true)
        .build()
    // decoder fallback: a phone's hardware H.265 decoder may stop at 1920x1088 (Snapdragon
    // 695), then the 2.5K picture is decoded by the software decoder of Android
    val renderers = DefaultRenderersFactory(context).setEnableDecoderFallback(true)
    return ExoPlayer.Builder(context, renderers).setLoadControl(load).build().apply {
        volume = 0f // the sound is off until the speaker button
    }
}
