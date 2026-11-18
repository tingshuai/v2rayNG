package com.v2ray.ang.handler

data class FreeNodeFeedbackReport(
    val candidate_id: String,
    val outcome: String,
    val latency_bucket: String,
) {
    companion object {
        fun from(candidateId: String?, delayMillis: Long): FreeNodeFeedbackReport? {
            if (candidateId == null || !Regex("[a-f0-9]{64}").matches(candidateId)) return null
            return if (delayMillis <= 0) {
                FreeNodeFeedbackReport(candidateId, "failure", "none")
            } else {
                val bucket = when {
                    delayMillis < 100 -> "lt_100"
                    delayMillis < 300 -> "100_300"
                    delayMillis < 1_000 -> "300_1000"
                    else -> "gte_1000"
                }
                FreeNodeFeedbackReport(candidateId, "success", bucket)
            }
        }
    }
}
