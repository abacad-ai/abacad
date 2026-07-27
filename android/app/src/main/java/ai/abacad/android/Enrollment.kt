package ai.abacad.android

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.util.concurrent.TimeUnit

/**
 * Device-first self-enrollment: the phone announces itself to a relay, receives
 * its permanent id and token, and shows that id plus a rotating claim code until
 * a human binds it to an account at <relay>/claim.
 *
 * Deliberately free of `android.*` imports so the whole thing runs — and is
 * tested — on a plain JVM. okhttp and org.json are ordinary Java libraries; the
 * Android-specific parts (SharedPreferences, Compose, the service) live at the
 * call sites. That matters here because this box can only *build* the APK, never
 * run it, so anything not covered by a JVM test is unverified.
 *
 * The wire contract is frozen against the Linux reference implementation
 * (linux/internal/enroll):
 *
 *   POST /api/devices/register        -> {device_id, device_token, claim_code, …}
 *   POST /api/devices/self/heartbeat  -> {claimed, claim_code, …} | {claimed, claimed_by}
 *
 * The token survives the claim unchanged, so unclaimed -> claimed needs no
 * re-key: the same credential that polls the heartbeat later dials /device.
 */
object Enrollment {

    const val DEFAULT_RELAY = "https://abacad.ai"

    /** Why an enrollment call failed, when the caller must react differently. */
    sealed class Failure : Exception() {
        /**
         * The relay doesn't recognize our token — the registration was reaped or
         * the device was deleted. Wipe the credential and register again.
         */
        object UnknownToken : Failure()

        /**
         * The relay predates self-enrollment. Fall back to the connection-URL
         * field rather than failing outright: someone self-hosting who upgrades
         * the app before the server should get a message, not a brick.
         */
        object NotSupported : Failure()

        data class Transport(val detail: String) : Failure()
    }

    data class Registration(
        val deviceId: String,
        val deviceToken: String,
        val claimCode: String,
        val claimExpiresIn: Int,
        val heartbeatIn: Int,
    )

    data class Status(
        val claimed: Boolean,
        val deviceId: String,
        val claimCode: String,
        val claimExpiresIn: Int,
        val heartbeatIn: Int,
        val name: String,
        /**
         * The account that claimed this device. Surfaced in the UI so a claim by
         * someone who merely read the screen isn't silent to the phone's owner.
         */
        val claimedBy: String,
    )

    private val http = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .build()

    private val JSON = "application/json".toMediaType()

    // ---- calls ----

    fun register(relay: String, platform: String, name: String, version: String): Registration {
        val body = JSONObject()
            .put("platform", platform).put("name", name).put("version", version)
            .toString().toRequestBody(JSON)
        val req = Request.Builder()
            .url(normalizeRelay(relay) + "/api/devices/register")
            .post(body)
            .build()
        val (code, text) = execute(req)
        if (code == 404 || code == 405) throw Failure.NotSupported
        if (code != 201 && code != 200) throw Failure.Transport(serverError(text, code))
        return parseRegistration(text)
    }

    fun heartbeat(relay: String, token: String): Status {
        val req = Request.Builder()
            .url(normalizeRelay(relay) + "/api/devices/self/heartbeat")
            // Header only: these endpoints are new, so there's no legacy ?token=
            // shape to support and no reason to put the secret in a URL.
            .header("Authorization", "Bearer $token")
            .post(ByteArray(0).toRequestBody(null))
            .build()
        val (code, text) = execute(req)
        if (code == 404 || code == 410 || code == 401) throw Failure.UnknownToken
        if (code != 200) throw Failure.Transport(serverError(text, code))
        return parseStatus(text)
    }

    private fun execute(req: Request): Pair<Int, String> =
        try {
            http.newCall(req).execute().use { resp ->
                Pair(resp.code, resp.body?.string().orEmpty())
            }
        } catch (e: Exception) {
            throw Failure.Transport(e.message ?: "network error")
        }

    // ---- pure helpers (unit-tested on the JVM) ----

    /** Parses a register response, rejecting one missing the fields we must persist. */
    fun parseRegistration(json: String): Registration {
        val o = JSONObject(json)
        val id = o.optString("device_id")
        val token = o.optString("device_token")
        if (id.isEmpty() || token.isEmpty()) {
            throw Failure.Transport("relay returned an incomplete registration")
        }
        return Registration(
            deviceId = id,
            deviceToken = token,
            claimCode = o.optString("claim_code"),
            claimExpiresIn = o.optInt("claim_expires_in"),
            heartbeatIn = o.optInt("heartbeat_in"),
        )
    }

    fun parseStatus(json: String): Status {
        val o = JSONObject(json)
        return Status(
            claimed = o.optBoolean("claimed"),
            deviceId = o.optString("device_id"),
            claimCode = o.optString("claim_code"),
            claimExpiresIn = o.optInt("claim_expires_in"),
            heartbeatIn = o.optInt("heartbeat_in"),
            name = o.optString("name"),
            claimedBy = o.optString("claimed_by"),
        )
    }

    fun normalizeRelay(relay: String): String {
        val s = relay.trim().trimEnd('/')
        return if (s.isEmpty()) DEFAULT_RELAY else s
    }

    /**
     * The WebSocket URL to dial once claimed. http(s) maps to ws(s), so a
     * loopback dev relay stays on plain ws while a real deployment gets wss —
     * matching DeviceClient's own refusal to speak ws:// off-loopback.
     */
    fun deviceUrl(relay: String): String {
        val base = normalizeRelay(relay)
        val ws = when {
            base.startsWith("https://") -> "wss://" + base.removePrefix("https://")
            base.startsWith("http://") -> "ws://" + base.removePrefix("http://")
            else -> base
        }
        return "$ws/device"
    }

    /** Clamp a server-advertised poll hint so a missing or hostile value can't spin us. */
    fun clampIntervalSec(seconds: Int): Long = when {
        seconds < 5 -> 20L
        seconds > 300 -> 300L
        else -> seconds.toLong()
    }

    private fun serverError(text: String, code: Int): String =
        try {
            JSONObject(text).optString("error").ifEmpty { "server said $code" }
        } catch (_: Exception) {
            "server said $code"
        }
}
