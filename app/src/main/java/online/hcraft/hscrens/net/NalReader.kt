package online.hcraft.hscrens.net

import java.io.DataInputStream
import java.io.InputStream

class NalReader(private val input: InputStream) {
    fun readNalUnits(onNal: (ByteArray) -> Unit) {
        val dataInput = DataInputStream(input)
        try {
            while (true) {
                val length = dataInput.readInt()
                if (length <= 0 || length > 10 * 1024 * 1024) break

                val nalWithStartCode = ByteArray(4 + length)
                nalWithStartCode[0] = 0.toByte()
                nalWithStartCode[1] = 0.toByte()
                nalWithStartCode[2] = 0.toByte()
                nalWithStartCode[3] = 1.toByte()

                dataInput.readFully(nalWithStartCode, 4, length)
                onNal(nalWithStartCode)
            }
        } catch (e: Exception) {
            // Socket encerrado ou erro de leitura
        }
    }
}
