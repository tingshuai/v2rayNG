package com.v2ray.ang.handler

import java.security.MessageDigest
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class FreeNodeFeedManifestTest {
    @Test
    fun `accepts versioned fresh manifest and checks its digest`() {
        val now = Instant.parse("2026-11-14T12:00:00Z")
        val id = "a".repeat(64)
        val revokedId = "b".repeat(64)
        val config = "trojan://secret@node.example:443"
        val canonical = "$id\t$config\n--revoked--\n$revokedId"
        val digest = MessageDigest.getInstance("SHA-256").digest(canonical.toByteArray()).joinToString("") { "%02x".format(it) }
        val json = """{"version":2,"generated_at":"2026-11-14T11:59:00Z","expires_at":"2026-11-14T13:59:00Z","content_sha256":"$digest","candidates":[{"id":"$id","config":"$config"}],"revoked":["$revokedId"]}"""

        val manifest = FreeNodeFeedManifest.parse(json, now)

        assertEquals(listOf(FreeNodeFeedCandidate(id, config)), manifest?.candidates)
        assertEquals(setOf(revokedId), manifest?.revoked)
    }

    @Test
    fun `rejects stale manifest and digest mismatch`() {
        val stale = """{"version":2,"generated_at":"2026-11-14T10:00:00Z","expires_at":"2026-11-14T11:00:00Z","content_sha256":"${"0".repeat(64)}","candidates":[],"revoked":[]}"""
        val badDigest = stale.replace("2026-11-14T11:00:00Z", "2026-11-14T13:00:00Z")
        val now = Instant.parse("2026-11-14T12:00:00Z")

        assertNull(FreeNodeFeedManifest.parse(stale, now))
        assertNull(FreeNodeFeedManifest.parse(badDigest, now))
    }
}
