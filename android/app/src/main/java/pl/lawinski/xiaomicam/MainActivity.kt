package pl.lawinski.xiaomicam

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.view.WindowManager
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.Videocam
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.focusGroup
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    /** a camera asked by a tap on the widget (a new object each time: shown again) */
    private val open = mutableStateOf<OpenCamera?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        // the picture on the whole screen: system bars come back with a swipe
        WindowCompat.getInsetsController(window, window.decorView).apply {
            systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            hide(WindowInsetsCompat.Type.systemBars())
        }
        val tv = packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)
        if (savedInstanceState == null) onIntent(intent)
        setContent { XcTheme { AppRoot(tv, open.value) } }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        onIntent(intent)
    }

    private fun onIntent(intent: Intent?) {
        intent?.getStringExtra(Widgets.EXTRA_CAMERA)?.let { open.value = OpenCamera(it) }
    }
}

class OpenCamera(val name: String)

private enum class Screen { LOADING, LIVE, GRID, WELCOME, LOGIN, DEVICES, IP, PICK, MENU }

const val PREFS = "xiaomi-cam"
const val PREF_SOUND = "sound" // the sound of the live view is on
private const val PREF_CAMERA = "camera"
private const val PREF_GRID = "grid" // the grid was shown last

@OptIn(ExperimentalComposeUiApi::class) // focusProperties.enter
@Composable
private fun AppRoot(tv: Boolean, open: OpenCamera?) {
    val s = strings()
    val context = LocalContext.current
    val prefs = remember { context.getSharedPreferences(PREFS, Context.MODE_PRIVATE) }
    val scope = rememberCoroutineScope()
    var screen by remember { mutableStateOf(Screen.LOADING) }
    var cameras by remember { mutableStateOf<List<Camera>>(emptyList()) }
    var selected by remember { mutableStateOf<Camera?>(null) }
    // the view under the menu and the camera list: LIVE or GRID. It stays composed under them,
    // so its video keeps playing (no reconnect to the camera after closing the menu).
    var base by remember { mutableStateOf(Screen.LIVE) }
    LaunchedEffect(screen) { if (screen == Screen.LIVE || screen == Screen.GRID) base = screen }

    fun toast(msg: String) = Toast.makeText(context, msg, Toast.LENGTH_LONG).show()

    // reload the camera list and show the camera (name: the one to select)
    fun reload(name: String? = null) {
        scope.launch {
            try {
                cameras = Core.cameras()
            } catch (e: Exception) {
                toast(s.error.format(e.message))
                cameras = emptyList()
            }
            val want = name ?: prefs.getString(PREF_CAMERA, null)
            selected = cameras.firstOrNull { it.name == want } ?: cameras.firstOrNull()
            selected?.let { prefs.edit().putString(PREF_CAMERA, it.name).apply() }
            screen = when {
                // the activity was recreated during the login (e.g. while reading the e-mail)
                name == null && LoginSession.active -> Screen.LOGIN
                selected == null -> Screen.WELCOME
                name == null && cameras.size > 1 && prefs.getBoolean(PREF_GRID, false) -> Screen.GRID
                else -> Screen.LIVE
            }
        }
    }
    LaunchedEffect(open) { reload(open?.name) }

    val importer = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            try {
                val yaml = context.contentResolver.openInputStream(uri)!!.use { it.readBytes().decodeToString() }
                Core.importConfig(yaml)
                toast(s.importDone)
                (context.applicationContext as XcApp).restart()
            } catch (e: Exception) {
                toast(s.error.format(e.message))
            }
        }
    }

    val hasCameras = cameras.isNotEmpty()
    fun showGrid(on: Boolean) {
        prefs.edit().putBoolean(PREF_GRID, on).apply()
        screen = if (on) Screen.GRID else Screen.LIVE
        base = screen
    }
    val back = {
        screen = when {
            !hasCameras -> Screen.WELCOME
            screen == Screen.MENU || screen == Screen.PICK -> base
            prefs.getBoolean(PREF_GRID, false) && cameras.size > 1 -> Screen.GRID
            else -> Screen.LIVE
        }
    }
    BackHandler(screen != Screen.LIVE && screen != Screen.GRID && screen != Screen.WELCOME && screen != Screen.LOADING) { back() }
    // Back in the single view opened from the grid: to the grid
    BackHandler(screen == Screen.LIVE && prefs.getBoolean(PREF_GRID, false) && cameras.size > 1) {
        screen = Screen.GRID
        base = Screen.GRID
    }

    val overMenu = screen == Screen.MENU || screen == Screen.PICK
    Box(Modifier.fillMaxSize()) {
    if (screen == Screen.LIVE || screen == Screen.GRID || overMenu) {
        // under the menu: no remote focus into it (the menu has the focus)
        Box(Modifier.fillMaxSize().focusProperties { if (overMenu) enter = { FocusRequester.Cancel } }.focusGroup()) {
            if (base == Screen.LIVE) selected?.let { cam ->
                LiveScreen(
                    camera = cam, tv = tv,
                    onPickCamera = { screen = Screen.PICK },
                    onGrid = if (cameras.size > 1) ({ showGrid(true) }) else null,
                    onMenu = { screen = Screen.MENU },
                    onError = { toast(s.error.format(it)) },
                )
            }
            else GridScreen(
                cameras = cameras,
                onOpen = { cam ->
                    selected = cam
                    prefs.edit().putString(PREF_CAMERA, cam.name).apply()
                    base = Screen.LIVE
                    screen = Screen.LIVE // Back returns to the grid
                },
                onSingle = { showGrid(false) },
                onMenu = { screen = Screen.MENU },
            )
        }
    }
    when (screen) {
        Screen.LOADING -> LoadingScreen()
        Screen.LIVE, Screen.GRID -> {}
        Screen.WELCOME -> WelcomeScreen(
            onLogin = {
                // a logged in account: straight to its cameras
                scope.launch { screen = if (Core.accounts().isNotEmpty()) Screen.DEVICES else Screen.LOGIN }
            },
            onImport = { importer.launch(arrayOf("*/*")) },
            onIp = { screen = Screen.IP },
            onCancel = if (hasCameras) back else null,
        )
        Screen.LOGIN -> LoginScreen(onDone = { screen = Screen.DEVICES }, onCancel = back)
        Screen.DEVICES -> DevicesScreen(onAdded = { reload(it) }, onCancel = back)
        Screen.IP -> IpCameraScreen(onAdded = { reload(it) }, onCancel = back)
        Screen.PICK -> CameraList(cameras, selected?.name) { cam ->
            selected = cam
            prefs.edit().putString(PREF_CAMERA, cam.name).apply()
            base = Screen.LIVE
            screen = Screen.LIVE
        }
        Screen.MENU -> selected?.let { cam ->
            MenuScreen(
                camera = cam,
                onAdd = { screen = Screen.WELCOME },
                onRenamed = { reload(it) },
                onRemoved = {
                    prefs.edit().remove(PREF_CAMERA).apply()
                    reload()
                },
                onClose = back,
            )
        } ?: run { screen = Screen.WELCOME }
    }
    }
}

@Composable
private fun CameraList(cameras: List<Camera>, selected: String?, onPick: (Camera) -> Unit) {
    val s = strings()
    FormScreen(s.cameras) {
        cameras.forEach { c ->
            Box(Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(Palette.surface)) {
                XcButton(
                    onClick = { onPick(c) }, icon = Icons.Rounded.Videocam,
                    text = if (c.info.isNotEmpty()) "${c.name}  ·  ${c.info}" else c.name,
                    active = c.name == selected, size = SizeLarge, shape = RoundedCornerShape(12.dp),
                    modifier = Modifier.fillMaxWidth(), alignStart = true,
                )
            }
        }
    }
}

@Composable
private fun MenuScreen(
    camera: Camera,
    onAdd: () -> Unit,
    onRenamed: (String) -> Unit,
    onRemoved: () -> Unit,
    onClose: () -> Unit,
) {
    val s = strings()
    val scope = rememberCoroutineScope()
    var renaming by remember { mutableStateOf(false) }
    var removing by remember { mutableStateOf(false) }
    var newName by remember { mutableStateOf(camera.name) }
    var error by remember { mutableStateOf<String?>(null) }

    FormScreen(camera.name) {
        when {
            renaming -> {
                OutlinedTextField(newName, { newName = it }, Modifier.fillMaxWidth(), label = { Text(s.ipName) }, singleLine = true)
                ErrorText(error)
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    Box(Modifier.weight(1f)) { BigButton(s.cancel, null) { renaming = false } }
                    Box(Modifier.weight(1f)) {
                        BigButton(s.save, null, primary = true) {
                            scope.launch {
                                try {
                                    Core.renameCamera(camera.name, newName)
                                    onRenamed(newName.trim())
                                } catch (e: Exception) {
                                    error = e.message
                                }
                            }
                        }
                    }
                }
            }
            removing -> {
                Text(s.removeAsk.format(camera.name), style = MaterialTheme.typography.bodyLarge)
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    Box(Modifier.weight(1f)) { BigButton(s.cancel, null) { removing = false } }
                    Box(Modifier.weight(1f)) {
                        BigButton(s.remove, Icons.Rounded.Delete, primary = true) {
                            scope.launch {
                                runCatching { Core.removeCamera(camera.name) }
                                onRemoved()
                            }
                        }
                    }
                }
            }
            else -> {
                BigButton(s.addCamera, Icons.Rounded.Add, onClick = onAdd)
                BigButton(s.rename, Icons.Rounded.Edit) { renaming = true }
                BigButton(s.remove, Icons.Rounded.Delete) { removing = true }
                BigButton(s.close, null, primary = true, onClick = onClose)
                Text(s.version.format(BuildConfig.VERSION_NAME), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
