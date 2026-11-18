package com.v2ray.ang.handler

import com.google.gson.JsonParser
import java.security.MessageDigest
import java.time.Instant

data class FreeNodeFeedCandidate(val id: String, val config: String)

data class FreeNodeFeedManifest(
    val version: Int,
    val generatedAt: Instant,
    val expiresAt: Instant,
    val contentSha256: String,
    val candidates: List<FreeNodeFeedCandidate>,
    val revoked: Set<String>,
) {
    companion object {
        private const val MAX_CANDIDATES = 32
        private val candidateId = Regex("[a-f0-9]{64}")

        fun parse(raw: String, now: Instant = Instant.now()): FreeNodeFeedManifest? = runCatching {
            val root = JsonParser.parseString(raw).takeIf { it.isJsonObject }?.asJsonObject ?: return null
            val version = root.get("version")?.asInt ?: return null
            val generatedAt = root.get("generated_at")?.asString?.let(Instant::parse) ?: return null
            val expiresAt = root.get("expires_at")?.asString?.let(Instant::parse) ?: return null
            val hash = root.get("content_sha256")?.asString ?: return null
            if (version != 2 || !Regex("[a-f0-9]{64}").matches(hash) || generatedAt.isAfter(now.plusSeconds(300)) || !expiresAt.isAfter(now) || expiresAt.isAfter(generatedAt.plusSeconds(2 * 60 * 60 + 300))) return null
            val candidateArray = root.get("candidates")?.takeIf { it.isJsonArray }?.asJsonArray ?: return null
            if (candidateArray.size() > MAX_CANDIDATES) return null
            val candidates = candidateArray.map { item ->
                if (!item.isJsonObject) return null
                val id = item.asJsonObject.get("id")?.asString ?: return null
                val config = item.asJsonObject.get("config")?.asString ?: return null
                if (!candidateId.matches(id) || !isSupportedCandidate(config)) return null
                FreeNodeFeedCandidate(id, config)
            }
            if (candidates.map { it.id }.toSet().size != candidates.size || candidates.map { it.config }.toSet().size != candidates.size) return null
            val revokedArray = root.get("revoked")?.takeIf { it.isJsonArray }?.asJsonArray ?: return null
            val revoked = revokedArray.map { it.asString }
            if (revoked.any { !candidateId.matches(it) } || revoked.toSet().size != revoked.size) return null
            val canonical = (candidates.map { "${it.id}\t${it.config}" } + "--revoked--" + revoked.sorted()).joinToString("\n")
            val actualHash = MessageDigest.getInstance("SHA-256").digest(canonical.toByteArray(Charsets.UTF_8)).joinToString("") { "%02x".format(it) }
            if (!actualHash.equals(hash, ignoreCase = true)) return null
            FreeNodeFeedManifest(version, generatedAt, expiresAt, hash, candidates, revoked.toSet())
        }.getOrNull()

        private fun isSupportedCandidate(config: String): Boolean {
            if (config.isBlank() || config.any { it.isWhitespace() }) return false
            val scheme = config.substringBefore(":", "").lowercase()
            return scheme in setOf("vless", "trojan", "ss") && config.contains("@")
        }
    }
}
