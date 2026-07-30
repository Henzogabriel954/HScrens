package online.hcraft.hscrens

import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.ActivityInfo
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.util.Log
import android.view.Surface
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.lifecycleScope
import online.hcraft.hscrens.input.TouchCapture
import online.hcraft.hscrens.net.ControlSocketClient
import online.hcraft.hscrens.net.NalReader
import online.hcraft.hscrens.net.ReconnectingSocket
import online.hcraft.hscrens.video.VideoDecoder

class MainActivity : ComponentActivity(), SurfaceHolder.Callback {

    private lateinit var surfaceView: SurfaceView
    private lateinit var touchCapture: TouchCapture

    private var videoSocket: ReconnectingSocket? = null
    private var touchSocket: ReconnectingSocket? = null
    private var controlSocket: ReconnectingSocket? = null
    private var videoDecoder: VideoDecoder? = null

    // Surface só fica disponível depois de surfaceCreated — guardamos aqui
    @Volatile private var activeSurface: Surface? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        enableImmersiveMode()
        setContentView(R.layout.activity_main)

        surfaceView = findViewById(R.id.surfaceView)
        touchCapture = TouchCapture(surfaceView)

        // Registra callback para receber surfaceCreated antes de iniciar sockets
        surfaceView.holder.addCallback(this)
    }

    // ── SurfaceHolder.Callback ────────────────────────────────────────────────

    override fun surfaceCreated(holder: SurfaceHolder) {
        Log.i("HScrens", "surfaceCreated — iniciando sockets")
        activeSurface = holder.surface
        startSockets()
    }

    override fun surfaceChanged(holder: SurfaceHolder, format: Int, width: Int, height: Int) {
        Log.i("HScrens", "surfaceChanged ${width}x${height}")
        activeSurface = holder.surface
    }

    override fun surfaceDestroyed(holder: SurfaceHolder) {
        Log.i("HScrens", "surfaceDestroyed — parando decoder")
        activeSurface = null
        videoDecoder?.stop()
        videoDecoder = null
    }

    // ── Sockets ───────────────────────────────────────────────────────────────

    private fun startSockets() {
        // Control Socket (porta 5100)
        controlSocket = ReconnectingSocket("127.0.0.1", 5100,
            onConnected = { socket ->
                Log.i("HScrens", "Control socket conectado")
                val metrics = resources.displayMetrics
                val client = ControlSocketClient(socket) { orientation ->
                    runOnUiThread { applyOrientation(orientation) }
                }
                client.sendHandshake(
                    getStableDeviceId(this),
                    Build.MODEL,
                    Build.MANUFACTURER,
                    metrics.widthPixels,
                    metrics.heightPixels,
                    metrics.densityDpi          // density real do aparelho
                )
                client.startListening()
            },
            onDisconnected = { Log.w("HScrens", "Control socket desconectado") }
        )
        controlSocket?.start(lifecycleScope)

        // Touch Socket (porta 5001)
        touchSocket = ReconnectingSocket("127.0.0.1", 5001,
            onConnected = { socket ->
                Log.i("HScrens", "Touch socket conectado")
                touchCapture.outputStream = socket.getOutputStream()
                try {
                    socket.getInputStream().read()
                } catch (e: Exception) {}
            },
            onDisconnected = {
                Log.w("HScrens", "Touch socket desconectado")
                touchCapture.outputStream = null
            }
        )
        touchSocket?.start(lifecycleScope)

        // Video Socket (porta 5000)
        videoSocket = ReconnectingSocket("127.0.0.1", 5000,
            onConnected = { socket ->
                Log.i("HScrens", "Video socket conectado — lendo NAL units")
                val surface = activeSurface
                if (surface == null || !surface.isValid) {
                    Log.e("HScrens", "Surface inválida ao conectar vídeo — aguardando próxima reconexão")
                    socket.close()
                    return@ReconnectingSocket
                }

                val reader = NalReader(socket.getInputStream())
                var sps: ByteArray? = null
                var pps: ByteArray? = null
                var hasReceivedFirstKeyframe = false

                var lastLogTime = System.currentTimeMillis()
                var pFrameCount = 0
                var idrCount = 0

                reader.readNalUnits { nal ->
                    val type = if (nal.size > 4) nal[4].toInt() and 0x1F else 0

                    if (type == 1) pFrameCount++
                    if (type == 5) idrCount++

                    val now = System.currentTimeMillis()
                    if (now - lastLogTime >= 1000) {
                        android.util.Log.i("HScrens", "📊 NAL Stats [Último 1s]: IDR (5) = $idrCount | P-frames (1) = $pFrameCount")
                        pFrameCount = 0
                        idrCount = 0
                        lastLogTime = now
                    }

                    var spsChanged = false
                    var ppsChanged = false

                    when (type) {
                        7 -> { 
                            if (sps == null || !nal.contentEquals(sps)) { sps = nal; spsChanged = true }
                            android.util.Log.i("HScrens", "✅ SPS (type 7) recebido: ${nal.size} bytes") 
                        }
                        8 -> { 
                            if (pps == null || !nal.contentEquals(pps)) { pps = nal; ppsChanged = true }
                            android.util.Log.i("HScrens", "✅ PPS (type 8) recebido: ${nal.size} bytes") 
                        }
                        5 -> { android.util.Log.d("HScrens", "🔑 Keyframe IDR (type 5) recebido: ${nal.size} bytes") }
                        1 -> { android.util.Log.v("HScrens", "🎞️ P-frame (type 1) recebido: ${nal.size} bytes") }
                    }

                    if (sps != null && pps != null) {
                        if (videoDecoder != null && (spsChanged || ppsChanged)) {
                            android.util.Log.w("HScrens", "🔄 SPS/PPS mudou (Resolução/Orientação)! Reiniciando decoder...")
                            videoDecoder?.stop()
                            videoDecoder = null
                            hasReceivedFirstKeyframe = false
                        }

                        if (videoDecoder == null) {
                            val s = activeSurface ?: return@readNalUnits
                            val metrics = resources.displayMetrics
                            val w = metrics.widthPixels
                            val h = metrics.heightPixels
                            android.util.Log.i("HScrens", "🚀 Inicializando VideoDecoder MediaCodec ${w}x${h}")
                            videoDecoder = VideoDecoder(s, w, h)
                            videoDecoder?.start(sps!!, pps!!)
                        }

                        // Só aceita pacotes no decoder após o primeiro IDR (5)
                        if (type == 5) hasReceivedFirstKeyframe = true

                        // Não envie SPS (7) e PPS (8) para o queueInputBuffer com flags 0.
                        // Envie apenas IDR (5) e frames normais (1).
                        if (hasReceivedFirstKeyframe && (type == 1 || type == 5)) {
                            videoDecoder?.queueNal(nal)
                        }
                    }
                }
            },
            onDisconnected = {
                Log.w("HScrens", "Video socket desconectado")
                videoDecoder?.stop()
                videoDecoder = null
            }
        )
        videoSocket?.start(lifecycleScope)
    }

    // ── UI helpers ────────────────────────────────────────────────────────────

    private fun enableImmersiveMode() {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        val controller = WindowInsetsControllerCompat(window, window.decorView)
        controller.hide(WindowInsetsCompat.Type.systemBars())
        controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (hasFocus) enableImmersiveMode()
    }

    fun applyOrientation(value: String) {
        requestedOrientation = when (value) {
            "landscape" -> ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
            else -> ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        }
    }

    @SuppressLint("HardwareIds")
    fun getStableDeviceId(context: Context): String {
        return Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID) ?: "unknown"
    }

    override fun onDestroy() {
        super.onDestroy()
        controlSocket?.stop()
        touchSocket?.stop()
        videoSocket?.stop()
        videoDecoder?.stop()
    }
}