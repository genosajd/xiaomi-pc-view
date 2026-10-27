package pl.lawinski.xiaomicam

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import pl.lawinski.xiaomicam.core.mobile.Mobile

// Core - the Go core (mobile/ in the repo, internal/xccore): the embedded go2rtc with the
// Xiaomi camera protocol. Every call may talk to the Xiaomi cloud or the camera, so all of
// them run on the IO dispatcher; lists come as JSON.

data class Camera(val name: String, val info: String, val xiaomi: Boolean)

data class Caps(
    val ptz: Boolean, val zoom: Boolean, val light: Boolean, val siren: Boolean,
    val points: Boolean, val history: Boolean, val nightMode: Boolean,
) {
    companion object {
        val NONE = Caps(false, false, false, false, false, false, false)
    }
}

/** playable: the decoder of this device can play the quality (size, codec) */
data class Qualities(val names: List<String>, val playable: List<Boolean>, val selected: Int)

data class Point(val slot: Int, val name: String, val saved: Boolean)

data class CamState(val light: Boolean?, val night: Int?)

data class NightMode(val value: Int, val id: String)

data class Device(val name: String, val info: String, val url: String)

data class LoginStep(
    val ok: Boolean, val captcha: String, val verifyTarget: String,
    val qr: String = "", val qrWait: Boolean = false,
)

object Core {
    const val MOTOR_LEFT = 1L
    const val MOTOR_RIGHT = 2L
    const val MOTOR_UP = 3L
    const val MOTOR_DOWN = 4L

    /** completes when go2rtc runs (XcApp starts it at process start) */
    val started = CompletableDeferred<Unit>()

    fun start(dir: String) {
        try {
            Mobile.start(dir)
            started.complete(Unit)
        } catch (e: Exception) {
            started.completeExceptionally(e)
        }
    }

    private suspend fun <T> io(block: () -> T): T {
        started.await()
        return withContext(Dispatchers.IO) { block() }
    }

    fun rtspUrl(name: String): String = Mobile.rtspurl(name)
    fun rtspUrlAudio(name: String): String = Mobile.rtspurlAudio(name)

    /** one key frame of the camera as an MP4 file (the widget picture) */
    suspend fun keyFrame(name: String, path: String) = io { Mobile.keyFrame(name, path) }

    suspend fun cameras(): List<Camera> = io {
        JSONArray(Mobile.cameras()).objects().map {
            Camera(it.getString("name"), it.optString("info"), it.optBoolean("xiaomi"))
        }
    }

    suspend fun caps(name: String): Caps = io {
        val o = JSONObject(Mobile.caps(name))
        Caps(
            o.optBoolean("PTZ"), o.optBoolean("Zoom"), o.optBoolean("Light"), o.optBoolean("Siren"),
            o.optBoolean("Points"), o.optBoolean("History"), o.optBoolean("NightMode"),
        )
    }

    suspend fun qualities(name: String): Qualities = io {
        val o = JSONObject(Mobile.qualities(name))
        val a = o.optJSONArray("names") ?: JSONArray()
        val sizes = o.optJSONArray("sizes") ?: JSONArray()
        val codecs = o.optJSONArray("codecs") ?: JSONArray()
        Qualities(
            (0 until a.length()).map { a.getString(it) },
            (0 until a.length()).map { i ->
                val wh = sizes.optJSONArray(i)
                Decoders.canPlay(codecs.optString(i), wh?.optInt(0) ?: 0, wh?.optInt(1) ?: 0)
            },
            o.optInt("selected"),
        )
    }

    suspend fun setQuality(name: String, i: Int) = io { Mobile.setQuality(name, i.toLong()) }

    // the motor: press starts a continuous move (repeated by the core every 200 ms), release
    // stops it; called from the main thread, the core runs it in its own goroutine
    fun motorPress(name: String, op: Long) = Mobile.motorPress(name, op)
    fun motorRelease() = Mobile.motorRelease()

    suspend fun zoom(name: String, z: Double) = io { Mobile.zoom(name, z) }
    fun currentZoom(name: String): Double = Mobile.currentZoom(name)

    suspend fun points(name: String): List<Point> = io {
        JSONArray(Mobile.points(name)).objects().map {
            Point(it.getInt("slot"), it.optString("name"), it.optBoolean("saved"))
        }
    }

    suspend fun gotoPoint(name: String, slot: Int) = io { Mobile.gotoPoint(name, slot.toLong()) }
    suspend fun savePoint(name: String, slot: Int, label: String) =
        io { Mobile.savePoint(name, slot.toLong(), label) }

    suspend fun state(name: String): CamState = io {
        val o = JSONObject(Mobile.state(name))
        CamState(
            if (o.has("light")) o.optBoolean("light") else null,
            if (o.has("night")) o.optInt("night") else null,
        )
    }

    suspend fun setLight(name: String, on: Boolean) = io { Mobile.setLight(name, on) }
    suspend fun setNightMode(name: String, value: Int) = io { Mobile.setNightMode(name, value.toLong()) }

    val nightModes: List<NightMode> by lazy {
        JSONArray(Mobile.nightModes()).objects().map { NightMode(it.getInt("Value"), it.getString("ID")) }
    }

    suspend fun removeCamera(name: String) = io { Mobile.removeCamera(name) }
    suspend fun renameCamera(name: String, newName: String) = io { Mobile.renameCamera(name, newName) }
    suspend fun addIPCamera(name: String, url: String): String = io { Mobile.addIPCamera(name, url) }

    // --- Mi account

    suspend fun accounts(): List<String> = io {
        val a = JSONArray(Mobile.accounts())
        (0 until a.length()).map { a.getString(it) }
    }

    /** region codes of the Mi cloud, in the order of the list */
    val regions: List<String> by lazy {
        JSONArray(Mobile.regions()).objects().map { it.getString("Code") }
    }

    private fun loginStep(json: String): LoginStep {
        val o = JSONObject(json)
        return LoginStep(
            o.optBoolean("ok"), o.optString("captcha"),
            o.optString("verify_email") + o.optString("verify_phone"),
            o.optString("qr"), o.optBoolean("qr_wait"),
        )
    }

    suspend fun login(user: String, password: String): LoginStep = io { loginStep(Mobile.login(user, password)) }
    suspend fun loginCode(key: String, code: String): LoginStep = io { loginStep(Mobile.loginCode(key, code)) }
    suspend fun loginQr(): LoginStep = io { loginStep(Mobile.loginQR()) }
    suspend fun loginQrWait(): LoginStep = io { loginStep(Mobile.loginQRWait()) }

    suspend fun devices(account: String, region: String): List<Device> = io {
        JSONArray(Mobile.devices(account, region)).objects().map {
            Device(it.getString("name"), it.optString("info"), it.getString("url"))
        }
    }

    suspend fun addXiaomi(d: Device): String = io { Mobile.addXiaomi(d.name, d.url) }

    suspend fun importConfig(yaml: String) = io { Mobile.importConfig(yaml) }
}

private fun JSONArray.objects(): List<JSONObject> = (0 until length()).map { getJSONObject(it) }
