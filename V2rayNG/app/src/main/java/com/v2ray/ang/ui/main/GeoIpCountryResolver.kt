package com.v2ray.ang.ui.main

import android.content.res.AssetManager
import java.net.InetAddress
import java.util.Locale
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/** Reads the Xray GeoIP asset for literal public IP addresses; hostnames are never resolved here. */
internal class GeoIpCountryResolver(private val assets: AssetManager) {
    private val mutex = Mutex()
    private val cache = mutableMapOf<String, String?>()

    suspend fun resolve(addresses: Collection<String>): Map<String, String> = mutex.withLock {
        val publicLiterals = addresses.distinct().mapNotNull { address ->
            publicAddress(address)?.let { address to it }
        }
        val missing = publicLiterals.filter { it.first !in cache }
        if (missing.isNotEmpty()) {
            val bytes = withContext(Dispatchers.IO) { assets.open("geoip.dat").use { it.readBytes() } }
            val found = withContext(Dispatchers.Default) { parseGeoIpCountries(bytes, missing.toMap()) }
            missing.forEach { (address, _) -> cache[address] = found[address] }
        }
        publicLiterals.mapNotNull { (address, _) -> cache[address]?.let { address to it } }.toMap()
    }
}

internal fun publicAddress(address: String): ByteArray? {
    val literal = address.trim().removePrefix("[").removeSuffix("]")
    if (literal.isEmpty() || literal.any { it !in "0123456789abcdefABCDEF:." }) return null
    if ('.' in literal && ':' !in literal && literal.split('.').size != 4) return null
    val ip = runCatching { InetAddress.getByName(literal) }.getOrNull() ?: return null
    if (ip.isAnyLocalAddress || ip.isLoopbackAddress || ip.isLinkLocalAddress || ip.isSiteLocalAddress || ip.isMulticastAddress) return null
    return ip.address
}

/** GeoIPList/GeoIP/CIDR wire fields from the Xray asset format. */
internal fun parseGeoIpCountries(data: ByteArray, addresses: Map<String, ByteArray>): Map<String, String> {
    val result = mutableMapOf<String, String>()
    val validCountries = Locale.getISOCountries().toSet()
    val outer = ProtoCursor(data, 0, data.size)
    while (outer.hasNext() && result.size < addresses.size) {
        val tag = outer.varint().toInt()
        if (tag == 10) {
            val entry = outer.message()
            var country: String? = null
            var inverse = false
            while (entry.hasNext()) {
                val entryTag = entry.varint().toInt()
                when (entryTag) {
                    10 -> country = entry.bytes().toString(Charsets.UTF_8).uppercase(Locale.ROOT)
                    24 -> inverse = entry.varint() != 0L
                    else -> entry.skip(entryTag)
                }
            }
            if (!inverse && country in validCountries) {
                val ranges = outer.previousMessage()
                while (ranges.hasNext()) {
                    when (val entryTag = ranges.varint().toInt()) {
                        18 -> {
                            val cidr = ranges.message()
                            var ip: ByteArray? = null
                            var prefix = -1
                            while (cidr.hasNext()) {
                                when (val cidrTag = cidr.varint().toInt()) {
                                    10 -> ip = cidr.bytes()
                                    16 -> prefix = cidr.varint().toInt()
                                    else -> cidr.skip(cidrTag)
                                }
                            }
                            if (ip != null && prefix >= 0) {
                                addresses.forEach { (address, target) ->
                                    if (address !in result && matchesCidr(target, ip, prefix)) result[address] = country!!
                                }
                            }
                        }
                        else -> ranges.skip(entryTag)
                    }
                }
            }
        } else outer.skip(tag)
    }
    return result
}

internal fun matchesCidr(address: ByteArray, network: ByteArray, prefix: Int): Boolean {
    if (address.size != network.size || prefix !in 0..(address.size * 8)) return false
    val fullBytes = prefix / 8
    for (index in 0 until fullBytes) if (address[index] != network[index]) return false
    val remaining = prefix % 8
    if (remaining == 0) return true
    val mask = 0xff shl (8 - remaining)
    return (address[fullBytes].toInt() and mask) == (network[fullBytes].toInt() and mask)
}

private class ProtoCursor(private val data: ByteArray, private var position: Int, private val end: Int) {
    private var previousMessageStart = position
    private var previousMessageEnd = position
    fun hasNext(): Boolean = position < end

    fun varint(): Long {
        var result = 0L
        for (shift in 0 until 64 step 7) {
            require(position < end) { "Truncated GeoIP varint" }
            val value = data[position++].toInt() and 0xff
            result = result or ((value and 0x7f).toLong() shl shift)
            if (value and 0x80 == 0) return result
        }
        error("Oversized GeoIP varint")
    }

    fun bytes(): ByteArray {
        val length = varint().toInt()
        require(length >= 0 && length <= end - position) { "Invalid GeoIP length" }
        return data.copyOfRange(position, position + length).also { position += length }
    }

    fun message(): ProtoCursor {
        val length = varint().toInt()
        require(length >= 0 && length <= end - position) { "Invalid GeoIP message" }
        previousMessageStart = position
        previousMessageEnd = position + length
        return ProtoCursor(data, position, position + length).also { position += length }
    }

    fun previousMessage(): ProtoCursor = ProtoCursor(data, previousMessageStart, previousMessageEnd)

    fun skip(tag: Int) {
        when (tag and 7) {
            0 -> varint()
            1 -> position += 8
            2 -> {
                val length = varint().toInt()
                position += length
            }
            5 -> position += 4
            else -> error("Unsupported GeoIP wire type")
        }
        require(position <= end) { "Truncated GeoIP field" }
    }
}
