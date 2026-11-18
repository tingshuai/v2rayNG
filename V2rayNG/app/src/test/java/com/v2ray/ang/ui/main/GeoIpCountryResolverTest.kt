package com.v2ray.ang.ui.main

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class GeoIpCountryResolverTest {
    @Test
    fun mapsOnlyMatchingPublicLiteralAddresses() {
        val cidr = field(1, byteArrayOf(1, 2, 3, 0)) + byteArrayOf(16, 24)
        val country = field(1, "US".toByteArray()) + field(2, cidr)
        val geoip = field(1, country)
        val addresses = mapOf(
            "1.2.3.4" to publicAddress("1.2.3.4")!!,
            "1.2.4.4" to publicAddress("1.2.4.4")!!,
        )
        assertEquals(mapOf("1.2.3.4" to "US"), parseGeoIpCountries(geoip, addresses))
        assertNull(publicAddress("example.com"))
        assertNull(publicAddress("127.0.0.1"))
        assertNull(publicAddress("192.168.1.1"))
    }

    @Test
    fun prefixComparisonRejectsWrongFamilyAndPrefix() {
        assertTrue(matchesCidr(byteArrayOf(1, 2, 3, 4), byteArrayOf(1, 2, 3, 0), 24))
        assertFalse(matchesCidr(byteArrayOf(1, 2, 4, 4), byteArrayOf(1, 2, 3, 0), 24))
        assertFalse(matchesCidr(byteArrayOf(1, 2, 3, 4), byteArrayOf(1, 2, 3, 0), 33))
    }

    private fun field(number: Int, bytes: ByteArray): ByteArray =
        byteArrayOf((number shl 3 or 2).toByte(), bytes.size.toByte()) + bytes
}
