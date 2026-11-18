package com.v2ray.ang.ui.main

import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.enums.EConfigType
import java.util.Locale
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MainListInteractionTest {
    private val profile = ProfileItem(configType = EConfigType.VLESS, remarks = "Tokyo-01", server = "203.0.113.8")

    @Test
    fun searchMatchesNodeNameAndLocalizedCountry() {
        assertTrue(serverMatchesSearch(profile, "JP", "tok", Locale.ENGLISH))
        assertTrue(serverMatchesSearch(profile, "JP", "本", Locale.SIMPLIFIED_CHINESE))
        assertTrue(serverMatchesSearch(profile, "JP", "japan", Locale.SIMPLIFIED_CHINESE))
        assertTrue(serverMatchesSearch(profile, "JP", "jp", Locale.ENGLISH))
        assertFalse(serverMatchesSearch(profile, "JP", "德国", Locale.SIMPLIFIED_CHINESE))
        assertFalse(serverMatchesSearch(profile, null, "日本", Locale.SIMPLIFIED_CHINESE))
    }

    @Test
    fun searchHandlesEmptyAndMalformedPattern() {
        assertTrue(serverMatchesSearch(profile, null, "   "))
        assertFalse(serverMatchesSearch(profile, "JP", "[", Locale.ENGLISH))
    }

    @Test
    fun latencyTonesFollowMeasuredDelayBoundaries() {
        assertEquals(LatencyTone.FAILED, latencyTone(-1))
        assertEquals(LatencyTone.UNKNOWN, latencyTone(0))
        assertEquals(LatencyTone.FAST, latencyTone(200))
        assertEquals(LatencyTone.MODERATE, latencyTone(201))
        assertEquals(LatencyTone.MODERATE, latencyTone(500))
        assertEquals(LatencyTone.SLOW, latencyTone(501))
    }
}
