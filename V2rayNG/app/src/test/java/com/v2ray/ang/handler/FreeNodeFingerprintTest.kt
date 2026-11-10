package com.v2ray.ang.handler

import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.enums.EConfigType
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Test

class FreeNodeFingerprintTest {
    private val key = ByteArray(32) { it.toByte() }

    @Test
    fun `fingerprint ignores subscription metadata but changes with connection fields`() {
        val original = ProfileItem.create(EConfigType.VLESS).apply {
            server = "example.com"
            serverPort = "443"
            username = "user-one"
            subscriptionId = "old-subscription"
            remarks = "old name"
        }
        val replacement = original.copy(
            subscriptionId = "new-subscription",
            remarks = "new name",
            addedTime = original.addedTime + 1,
        )
        val changedConnection = replacement.copy(serverPort = "8443")

        assertEquals(
            FreeNodeFingerprint.create(original, key),
            FreeNodeFingerprint.create(replacement, key),
        )
        assertNotEquals(
            FreeNodeFingerprint.create(original, key),
            FreeNodeFingerprint.create(changedConnection, key),
        )
    }
}
