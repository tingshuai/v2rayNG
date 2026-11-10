package com.v2ray.ang.handler

import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.util.JsonUtil
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/** Produces a device-keyed identifier without persisting connection secrets. */
object FreeNodeFingerprint {
    fun create(profile: ProfileItem, key: ByteArray): String {
        require(key.size >= 32) { "free-node fingerprint key must be at least 32 bytes" }
        val canonical = JsonUtil.toJson(profile.duplicateIdentity())
        val mac = Mac.getInstance("HmacSHA256")
        mac.init(SecretKeySpec(key, "HmacSHA256"))
        return mac.doFinal(canonical.toByteArray(Charsets.UTF_8)).joinToString("") { "%02x".format(it) }
    }
}
