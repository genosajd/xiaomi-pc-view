package pl.lawinski.xiaomicam

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder

/**
 * Keeps the process (and so the embedded go2rtc: API 127.0.0.1:11984, RTSP 127.0.0.1:18554,
 * started in XcApp) alive in the background, so another app on the device can play the cameras,
 * e.g. the "Kokpit F11" car launcher. Off by default (pref `background_server`); the service
 * itself only holds a foreground notification, go2rtc starts with the process.
 */
class ServerService : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (!Server.enabled(this)) {
            stopSelf()
            return START_NOT_STICKY
        }
        val nm = getSystemService(NotificationManager::class.java)
        val s = stringsFor(this)
        nm.createNotificationChannel(NotificationChannel(CHANNEL, s.serverChannel, NotificationManager.IMPORTANCE_MIN))
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE,
        )
        val n = Notification.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_notify_camera)
            .setContentTitle(s.serverChannel)
            .setContentText(s.serverNote)
            .setContentIntent(open)
            .setOngoing(true)
            .build()
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTE_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTE_ID, n)
        }
        return START_STICKY
    }

    companion object {
        private const val CHANNEL = "server"
        private const val NOTE_ID = 7
    }
}

object Server {
    const val PREF = "background_server"

    /** sent by another app to wake the process (go2rtc starts with it) */
    const val ACTION_WAKE = "pl.lawinski.xiaomicam.action.WAKE_SERVER"

    fun enabled(ctx: Context) = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean(PREF, false)

    fun setEnabled(ctx: Context, on: Boolean) {
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putBoolean(PREF, on).apply()
        sync(ctx)
    }

    /** starts or stops the foreground service to match the setting */
    fun sync(ctx: Context) {
        val intent = Intent(ctx, ServerService::class.java)
        if (enabled(ctx)) {
            // a start from the background can be refused (Android 12+); the process still runs
            // for a while after a wake, and the next start from the app or a boot succeeds
            runCatching { ctx.startForegroundService(intent) }
        } else {
            ctx.stopService(intent)
        }
    }
}

/** boot: the service again (if it is on); a wake from another app: the process is up already */
class ServerReceiver : BroadcastReceiver() {
    override fun onReceive(ctx: Context, intent: Intent) {
        when (intent.action) {
            Intent.ACTION_BOOT_COMPLETED, Intent.ACTION_MY_PACKAGE_REPLACED, Server.ACTION_WAKE -> Server.sync(ctx)
        }
    }
}
