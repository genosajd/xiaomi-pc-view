package pl.lawinski.xiaomicam

import android.app.Activity
import android.app.PendingIntent
import android.app.job.JobInfo
import android.app.job.JobParameters
import android.app.job.JobScheduler
import android.app.job.JobService
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.media.MediaMetadataRetriever
import android.os.Build
import android.os.Bundle
import android.os.PersistableBundle
import android.view.View
import android.widget.RemoteViews
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Videocam
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.text.DateFormat
import java.util.Date

// The home screen widget: the last picture of one camera, resizable (the picture is cropped
// to the widget). A widget can't play video, so the picture is one key frame of the camera
// (go2rtc api/frame.mp4, decoded by MediaMetadataRetriever), taken by WidgetJob: on the
// refresh button and every 15/30/60 min (or only by hand). Each picture opens the camera
// session for a few seconds (the camera takes one session at a time). Tap: the app on this
// camera.
object Widgets {
    const val EXTRA_CAMERA = "pl.lawinski.xiaomicam.CAMERA"
    const val ACTION_REFRESH = "pl.lawinski.xiaomicam.WIDGET_REFRESH"

    /** refresh intervals in minutes; 0: only with the refresh button */
    val intervals = listOf(0, 15, 30, 60)
    const val DEFAULT_INTERVAL = 30

    private const val JOB_PERIODIC = 1
    private const val JOB_NOW = 1000 // + the widget id
    private const val MAX_WIDTH = 1080 // px of the picture (RemoteViews bitmap memory)

    private fun prefs(ctx: Context) = ctx.getSharedPreferences("widgets", Context.MODE_PRIVATE)

    fun camera(ctx: Context, id: Int): String? = prefs(ctx).getString("$id.camera", null)
    fun interval(ctx: Context, id: Int): Int = prefs(ctx).getInt("$id.interval", DEFAULT_INTERVAL)

    fun save(ctx: Context, id: Int, camera: String, interval: Int) {
        prefs(ctx).edit().putString("$id.camera", camera).putInt("$id.interval", interval).apply()
    }

    fun remove(ctx: Context, ids: IntArray) {
        val e = prefs(ctx).edit()
        ids.forEach { e.remove("$it.camera").remove("$it.interval") }
        e.apply()
    }

    fun ids(ctx: Context): IntArray =
        AppWidgetManager.getInstance(ctx).getAppWidgetIds(ComponentName(ctx, CameraWidget::class.java))

    private fun fileName(camera: String) = camera.replace(Regex("[^\\p{L}\\p{N}_-]"), "_")
    fun picture(ctx: Context, camera: String) = File(ctx.filesDir, "widget/${fileName(camera)}.jpg")
    private fun failed(ctx: Context, camera: String) = prefs(ctx).getBoolean("$camera.failed", false)

    /** shows the saved picture of the widget's camera; refreshing: a spinner instead of the button */
    fun render(ctx: Context, id: Int, refreshing: Boolean = false) {
        val s = stringsFor(ctx)
        val v = RemoteViews(ctx.packageName, R.layout.widget_camera)
        val camera = camera(ctx, id)
        v.setTextViewText(R.id.widget_name, camera ?: s.widgetPick)
        val pic = camera?.let { picture(ctx, it) }
        val bmp = pic?.takeIf { it.exists() }?.let { BitmapFactory.decodeFile(it.path) }
        if (bmp != null) {
            v.setImageViewBitmap(R.id.widget_picture, bmp)
            v.setViewVisibility(R.id.widget_empty, View.GONE)
        } else {
            v.setViewVisibility(R.id.widget_empty, View.VISIBLE)
            v.setTextViewText(R.id.widget_empty, if (camera == null) s.widgetPick else s.widgetNoPicture)
        }
        v.setTextViewText(
            R.id.widget_time,
            when {
                camera != null && failed(ctx, camera) -> s.noVideo
                pic != null && pic.exists() -> DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(pic.lastModified()))
                else -> ""
            },
        )
        v.setViewVisibility(R.id.widget_refresh, if (refreshing || camera == null) View.GONE else View.VISIBLE)
        v.setViewVisibility(R.id.widget_progress, if (refreshing) View.VISIBLE else View.GONE)
        v.setContentDescription(R.id.widget_refresh, s.widgetRefresh)

        val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        val open = Intent(ctx, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            .putExtra(EXTRA_CAMERA, camera)
        v.setOnClickPendingIntent(R.id.widget_root, PendingIntent.getActivity(ctx, id, open, flags))
        val refresh = Intent(ctx, CameraWidget::class.java)
            .setAction(ACTION_REFRESH)
            .putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id)
        v.setOnClickPendingIntent(R.id.widget_refresh, PendingIntent.getBroadcast(ctx, id, refresh, flags))
        AppWidgetManager.getInstance(ctx).updateAppWidget(id, v)
    }

    /** takes a new picture for the widget now */
    fun refreshNow(ctx: Context, id: Int) {
        val extras = PersistableBundle().apply { putIntArray("ids", intArrayOf(id)) }
        val job = JobInfo.Builder(JOB_NOW + id, ComponentName(ctx, WidgetJob::class.java))
            .setExtras(extras)
            .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) job.setExpedited(true) else job.setOverrideDeadline(0)
        ctx.getSystemService(JobScheduler::class.java).schedule(job.build())
    }

    /** the periodic job runs while a widget refreshes by itself */
    fun schedule(ctx: Context) {
        val js = ctx.getSystemService(JobScheduler::class.java)
        if (ids(ctx).none { interval(ctx, it) > 0 && camera(ctx, it) != null }) {
            js.cancel(JOB_PERIODIC)
            return
        }
        if (js.getPendingJob(JOB_PERIODIC) != null) return
        js.schedule(
            JobInfo.Builder(JOB_PERIODIC, ComponentName(ctx, WidgetJob::class.java))
                .setPeriodic(15 * 60_000L)
                .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
                .setPersisted(true)
                .build()
        )
    }

    /** the widgets whose picture is older than their interval (2 min early: the job is inexact) */
    fun due(ctx: Context): IntArray = ids(ctx).filter { id ->
        val camera = camera(ctx, id) ?: return@filter false
        val every = interval(ctx, id)
        every > 0 && System.currentTimeMillis() - picture(ctx, camera).lastModified() >= (every - 2) * 60_000L
    }.toIntArray()

    /** a key frame of the camera → the saved picture (scaled down to MAX_WIDTH) */
    suspend fun takePicture(ctx: Context, camera: String) {
        val mp4 = File(ctx.cacheDir, "widget-${fileName(camera)}.mp4")
        try {
            Core.keyFrame(camera, mp4.path)
            withContext(Dispatchers.IO) {
                val r = MediaMetadataRetriever()
                val frame = try {
                    r.setDataSource(mp4.path)
                    r.getFrameAtTime(0, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
                } finally {
                    r.release()
                } ?: error("no frame")
                val bmp = if (frame.width > MAX_WIDTH) {
                    Bitmap.createScaledBitmap(frame, MAX_WIDTH, frame.height * MAX_WIDTH / frame.width, true)
                } else frame
                val out = picture(ctx, camera)
                out.parentFile?.mkdirs()
                val tmp = File(out.path + ".tmp")
                tmp.outputStream().use { bmp.compress(Bitmap.CompressFormat.JPEG, 85, it) }
                tmp.renameTo(out)
            }
            prefs(ctx).edit().putBoolean("$camera.failed", false).apply()
        } catch (e: Exception) {
            prefs(ctx).edit().putBoolean("$camera.failed", true).apply()
            throw e
        } finally {
            mp4.delete()
        }
    }
}

class CameraWidget : AppWidgetProvider() {
    override fun onUpdate(ctx: Context, manager: AppWidgetManager, ids: IntArray) {
        ids.forEach { Widgets.render(ctx, it) }
        Widgets.schedule(ctx)
    }

    override fun onReceive(ctx: Context, intent: Intent) {
        if (intent.action == Widgets.ACTION_REFRESH) {
            val id = intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID)
            if (id != AppWidgetManager.INVALID_APPWIDGET_ID) {
                Widgets.render(ctx, id, refreshing = true)
                Widgets.refreshNow(ctx, id)
            }
            return
        }
        super.onReceive(ctx, intent)
    }

    override fun onDeleted(ctx: Context, ids: IntArray) {
        Widgets.remove(ctx, ids)
        Widgets.schedule(ctx)
    }
}

/** takes the pictures: the widgets of the extras "ids", or (periodic) the due ones */
class WidgetJob : JobService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    override fun onStartJob(params: JobParameters): Boolean {
        val ids = params.extras.getIntArray("ids") ?: Widgets.due(this)
        if (ids.isEmpty()) return false
        scope.launch {
            // one picture per camera, shown on all its widgets
            ids.groupBy { Widgets.camera(this@WidgetJob, it) }.forEach { (camera, widgets) ->
                if (camera == null) return@forEach
                widgets.forEach { Widgets.render(this@WidgetJob, it, refreshing = true) }
                runCatching { Widgets.takePicture(this@WidgetJob, camera) }
                widgets.forEach { Widgets.render(this@WidgetJob, it) }
            }
            jobFinished(params, false)
        }
        return true
    }

    override fun onStopJob(params: JobParameters): Boolean {
        scope.cancel()
        (params.extras.getIntArray("ids") ?: Widgets.ids(this)).forEach { Widgets.render(this, it) }
        return false
    }
}

/** shown when a widget is added (or reconfigured): the camera and the refresh interval */
class WidgetConfigActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val id = intent?.extras?.getInt(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID)
            ?: AppWidgetManager.INVALID_APPWIDGET_ID
        setResult(Activity.RESULT_CANCELED, Intent().putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id))
        if (id == AppWidgetManager.INVALID_APPWIDGET_ID) {
            finish()
            return
        }
        setContent {
            XcTheme {
                WidgetSetup(id) { camera, interval ->
                    Widgets.save(this, id, camera, interval)
                    Widgets.render(this, id, refreshing = true)
                    Widgets.refreshNow(this, id)
                    Widgets.schedule(this)
                    setResult(Activity.RESULT_OK, Intent().putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id))
                    finish()
                }
            }
        }
    }
}

@Composable
private fun WidgetSetup(id: Int, onDone: (String, Int) -> Unit) {
    val s = strings()
    val ctx = androidx.compose.ui.platform.LocalContext.current
    var cameras by remember { mutableStateOf<List<Camera>?>(null) }
    var selected by remember { mutableStateOf(Widgets.camera(ctx, id)) }
    var interval by remember { mutableIntStateOf(Widgets.interval(ctx, id)) }
    LaunchedEffect(Unit) {
        cameras = runCatching { Core.cameras() }.getOrDefault(emptyList())
        if (selected == null || cameras!!.none { it.name == selected }) selected = cameras!!.firstOrNull()?.name
    }
    FormScreen(s.widgetTitle) {
        val list = cameras
        when {
            list == null -> Text(s.loading, color = Palette.muted)
            list.isEmpty() -> {
                Text(s.widgetNoCameras, style = MaterialTheme.typography.bodyLarge, color = Palette.muted)
                BigButton(s.close, null, primary = true) { (ctx as Activity).finish() }
            }
            else -> {
                Text(s.cameras, style = MaterialTheme.typography.titleMedium)
                list.forEach { c ->
                    Box(Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(Palette.surface)) {
                        XcButton(
                            onClick = { selected = c.name }, icon = Icons.Rounded.Videocam, text = c.name,
                            active = c.name == selected, size = SizeLarge, shape = RoundedCornerShape(12.dp),
                            modifier = Modifier.fillMaxWidth(), alignStart = true,
                        )
                    }
                }
                Text(s.widgetEvery, style = MaterialTheme.typography.titleMedium)
                Pill {
                    Row {
                        Widgets.intervals.forEach { m ->
                            XcButton(onClick = { interval = m }, text = s.widgetInterval(m), active = m == interval)
                        }
                    }
                }
                Text(s.widgetNote, style = MaterialTheme.typography.bodyMedium, color = Palette.muted)
                BigButton(s.add, Icons.Rounded.Add, primary = true) { selected?.let { onDone(it, interval) } }
            }
        }
    }
}
