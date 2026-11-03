package pl.lawinski.xiaomicam

import android.app.Application
import kotlin.concurrent.thread

class XcApp : Application() {
    override fun onCreate() {
        super.onCreate()
        // go2rtc reads its config and opens the local API/RTSP ports: off the main thread
        thread(name = "xccore-start") { Core.start(filesDir.absolutePath) }
        Server.sync(this)
    }

    /** restarts the process (go2rtc reads the config only at start: after an import) */
    fun restart() {
        val intent = packageManager.getLaunchIntentForPackage(packageName) ?: return
        intent.addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK or android.content.Intent.FLAG_ACTIVITY_CLEAR_TASK)
        startActivity(intent)
        Runtime.getRuntime().exit(0)
    }
}
