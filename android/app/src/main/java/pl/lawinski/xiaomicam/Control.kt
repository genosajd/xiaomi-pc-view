package pl.lawinski.xiaomicam

import android.content.ContentProvider
import android.content.ContentValues
import android.database.Cursor
import android.net.Uri
import android.os.Bundle
import kotlinx.coroutines.runBlocking
import org.json.JSONArray
import org.json.JSONObject

/**
 * Camera control for the user's car launcher "Kokpit F11" (its AI assistant turns the camera).
 * A ContentProvider.call() entry, so the caller's package is known: only the allowed packages get
 * an answer, everybody else a SecurityException. Video still goes through the local go2rtc.
 *
 * Methods (arg = camera name, extras as listed; result Bundle "ok" + "result" or "error"):
 *  - cameras: JSON [{"name","ptz","zoom","points","xiaomi"}]
 *  - move: extras "direction" (left/right/up/down), "ms" (100-4000) – presses the motor that long
 *  - points: JSON [{"slot","name","saved"}]
 *  - goto: extras "slot" (1-3)
 *  - zoom: extras "value" (1.0-9.0)
 */
class ControlProvider : ContentProvider() {
    override fun onCreate() = true

    override fun call(method: String, arg: String?, extras: Bundle?): Bundle {
        val caller = callingPackage
        if (caller !in ALLOWED) throw SecurityException("Camera control is only for $ALLOWED, not $caller")
        return try {
            Bundle().apply {
                putBoolean("ok", true)
                putString("result", run(method, arg.orEmpty(), extras ?: Bundle()))
            }
        } catch (e: Exception) {
            Bundle().apply {
                putBoolean("ok", false)
                putString("error", e.message ?: e.toString())
            }
        }
    }

    private fun run(method: String, name: String, ex: Bundle): String = runBlocking {
        Core.started.await()
        when (method) {
            "cameras" -> JSONArray().apply {
                Core.cameras().forEach { c ->
                    val caps = Core.caps(c.name)
                    put(JSONObject().put("name", c.name).put("xiaomi", c.xiaomi)
                        .put("ptz", caps.ptz).put("zoom", caps.zoom).put("points", caps.points))
                }
            }.toString()
            "move" -> {
                val op = when (ex.getString("direction")) {
                    "left" -> Core.MOTOR_LEFT
                    "right" -> Core.MOTOR_RIGHT
                    "up" -> Core.MOTOR_UP
                    "down" -> Core.MOTOR_DOWN
                    else -> error("direction must be left/right/up/down")
                }
                require(Core.caps(name).ptz) { "camera $name has no pan/tilt" }
                val ms = ex.getInt("ms", 800).coerceIn(100, 4000)
                Core.motorPress(name, op)
                try {
                    Thread.sleep(ms.toLong())
                } finally {
                    Core.motorRelease()
                }
                "moved ${ex.getString("direction")} for $ms ms"
            }
            "points" -> JSONArray().apply {
                Core.points(name).forEach { put(JSONObject().put("slot", it.slot).put("name", it.name).put("saved", it.saved)) }
            }.toString()
            "goto" -> {
                val slot = ex.getInt("slot")
                require(slot in 1..3) { "slot must be 1-3" }
                Core.gotoPoint(name, slot)
                "going to point $slot"
            }
            "zoom" -> {
                val z = ex.getDouble("value", 1.0).coerceIn(1.0, 9.0)
                Core.zoom(name, z)
                "zoom $z"
            }
            else -> error("unknown method $method")
        }
    }

    override fun query(uri: Uri, p: Array<out String>?, s: String?, a: Array<out String>?, o: String?): Cursor? = null
    override fun getType(uri: Uri): String? = null
    override fun insert(uri: Uri, values: ContentValues?): Uri? = null
    override fun delete(uri: Uri, s: String?, a: Array<out String>?) = 0
    override fun update(uri: Uri, values: ContentValues?, s: String?, a: Array<out String>?) = 0

    companion object {
        const val AUTHORITY = "pl.lawinski.xiaomicam.control"

        /** The car launcher Kokpit F11 (github.com/genosajd/kokpit). */
        val ALLOWED = setOf("pl.lawinski.radiolauncher")
    }
}
