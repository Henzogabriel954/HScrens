package online.hcraft.hscrens.net

import org.json.JSONObject
import java.io.BufferedReader
import java.io.BufferedWriter
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.Socket
import kotlin.concurrent.thread

class ControlSocketClient(
    private val socket: Socket,
    private val onOrientationChange: (String) -> Unit
) {
    private val reader = BufferedReader(InputStreamReader(socket.getInputStream()))
    private val writer = BufferedWriter(OutputStreamWriter(socket.getOutputStream()))

    fun startListening() {
        // Bloqueia e lê
        try {
            while (true) {
                val line = reader.readLine() ?: break
                val json = JSONObject(line)
                handleMessage(json)
            }
        } catch (e: Exception) {
            // Fim
        }
    }

    private fun handleMessage(json: JSONObject) {
        when (json.optString("type")) {
            "set_orientation" -> {
                val value = json.optString("value")
                onOrientationChange(value)
            }
            "ping" -> {
                val ts = json.optLong("ts")
                sendPong(ts)
            }
        }
    }

    fun sendHandshake(deviceId: String, model: String, manufacturer: String, width: Int, height: Int, densityDpi: Int) {
        val json = JSONObject().apply {
            put("type", "handshake")
            put("device_id", deviceId)
            put("model", model)
            put("manufacturer", manufacturer)
            put("native_width", width)
            put("native_height", height)
            put("density_dpi", densityDpi)
            put("app_version", 3)
        }
        writeLine(json.toString())
    }

    private fun sendPong(ts: Long) {
        val json = JSONObject().apply {
            put("type", "pong")
            put("ts", ts)
        }
        writeLine(json.toString())
    }

    @Synchronized
    private fun writeLine(line: String) {
        try {
            writer.write(line + "\n")
            writer.flush()
        } catch (e: Exception) {}
    }
}
