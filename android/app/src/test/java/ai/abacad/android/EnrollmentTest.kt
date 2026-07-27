package ai.abacad.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * JVM tests for the parts of enrollment that don't need a device.
 *
 * This box can build the APK but never run it, so anything not covered here is
 * unverified — which is exactly why Enrollment is free of `android.*` imports.
 */
class EnrollmentTest {

    @Test
    fun `parses a registration`() {
        val r = Enrollment.parseRegistration(
            """{"device_id":"abcdefghijklmnop","device_token":"abd_dev_x",
                "claim_code":"WXYZ-2K7M","claim_expires_in":300,"heartbeat_in":20}""")
        assertEquals("abcdefghijklmnop", r.deviceId)
        assertEquals("abd_dev_x", r.deviceToken)
        assertEquals("WXYZ-2K7M", r.claimCode)
        assertEquals(300, r.claimExpiresIn)
        assertEquals(20, r.heartbeatIn)
    }

    /**
     * A malformed success response must not be mistaken for a good one: the
     * client would persist an empty token and never recover.
     */
    @Test
    fun `rejects a registration with no id or token`() {
        assertThrows(Enrollment.Failure.Transport::class.java) {
            Enrollment.parseRegistration("""{"claim_code":"WXYZ-2K7M"}""")
        }
        assertThrows(Enrollment.Failure.Transport::class.java) {
            Enrollment.parseRegistration("""{"device_id":"abcdefghijklmnop"}""")
        }
    }

    @Test
    fun `parses unclaimed and claimed heartbeats`() {
        val pending = Enrollment.parseStatus(
            """{"claimed":false,"device_id":"abcdefghijklmnop","claim_code":"WXYZ-2K7M",
                "claim_expires_in":280,"heartbeat_in":20}""")
        assertEquals(false, pending.claimed)
        assertEquals("WXYZ-2K7M", pending.claimCode)

        val claimed = Enrollment.parseStatus(
            """{"claimed":true,"device_id":"abcdefghijklmnop","name":"drawer phone",
                "claimed_by":"ana@example.com"}""")
        assertTrue(claimed.claimed)
        // The disclosure that makes a shoulder-surf claim visible to the owner.
        assertEquals("ana@example.com", claimed.claimedBy)
        assertEquals("drawer phone", claimed.name)
    }

    @Test
    fun `relay normalization`() {
        assertEquals("https://abacad.ai", Enrollment.normalizeRelay(""))
        assertEquals("https://abacad.ai", Enrollment.normalizeRelay("  "))
        assertEquals("https://abacad.ai", Enrollment.normalizeRelay("https://abacad.ai/"))
        assertEquals("https://relay.example.com", Enrollment.normalizeRelay(" https://relay.example.com "))
    }

    @Test
    fun `device url maps scheme`() {
        assertEquals("wss://abacad.ai/device", Enrollment.deviceUrl("https://abacad.ai"))
        assertEquals("wss://abacad.ai/device", Enrollment.deviceUrl("https://abacad.ai/"))
        assertEquals("ws://10.0.2.2:8848/device", Enrollment.deviceUrl("http://10.0.2.2:8848"))
        assertEquals("wss://relay.example.com/device", Enrollment.deviceUrl("wss://relay.example.com"))
        assertEquals("wss://abacad.ai/device", Enrollment.deviceUrl(""))
    }

    /** A missing or hostile poll hint must not turn into a busy loop. */
    @Test
    fun `interval is clamped`() {
        assertEquals(20L, Enrollment.clampIntervalSec(0))
        assertEquals(20L, Enrollment.clampIntervalSec(-5))
        assertEquals(20L, Enrollment.clampIntervalSec(1))
        assertEquals(30L, Enrollment.clampIntervalSec(30))
        assertEquals(300L, Enrollment.clampIntervalSec(99999))
    }
}
