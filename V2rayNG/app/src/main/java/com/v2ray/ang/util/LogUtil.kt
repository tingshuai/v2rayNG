package com.v2ray.ang.util

import android.util.Log
import com.v2ray.ang.AppConfig
import com.v2ray.ang.handler.MmkvManager
import java.security.MessageDigest
import java.util.Locale

object LogUtil {

    private const val DEFAULT_LEVEL = "warning"
    private const val CACHE_UNSET = Int.MIN_VALUE

    @Volatile
    private var cachedMinPriority: Int = CACHE_UNSET

    private fun parsePriority(level: String?): Int {
        return when ((level ?: DEFAULT_LEVEL).lowercase(Locale.US)) {
            "verbose" -> Log.VERBOSE
            "debug" -> Log.DEBUG
            "info" -> Log.INFO
            "warn", "warning" -> Log.WARN
            "error" -> Log.ERROR
            "none", "off" -> Int.MAX_VALUE
            else -> Log.WARN
        }
    }

    @Suppress("unused")
    fun refreshLogLevel() {
        cachedMinPriority = parsePriority(MmkvManager.decodeSettingsString(AppConfig.PREF_LOGLEVEL, DEFAULT_LEVEL))
    }

    private fun minPriority(): Int {
        val cached = cachedMinPriority
        if (cached != CACHE_UNSET) {
            return cached
        }

        return synchronized(this) {
            val current = cachedMinPriority
            if (current != CACHE_UNSET) {
                current
            } else {
                parsePriority(MmkvManager.decodeSettingsString(AppConfig.PREF_LOGLEVEL, DEFAULT_LEVEL)).also {
                    cachedMinPriority = it
                }
            }
        }
    }

    private fun isEnabled(priority: Int): Boolean {
        return priority >= minPriority()
    }

    private fun log(priority: Int, tag: String, message: String, throwable: Throwable? = null) {
        if (!isEnabled(priority)) return

        when {
            throwable == null -> Log.println(priority, tag, message)
            priority >= Log.ERROR -> Log.e(tag, message, throwable)
            priority == Log.WARN -> Log.w(tag, message, throwable)
            priority == Log.INFO -> Log.i(tag, message, throwable)
            priority == Log.DEBUG -> Log.d(tag, message, throwable)
            else -> Log.v(tag, message, throwable)
        }
    }

    /** Writes bounded, secret-safe routing diagnostics to Android's rotating logcat buffers. */
    fun diagnostic(event: String, fields: Map<String, String>) {
        val safeEvent = event.takeIf { it.matches(Regex("[a-z_]{1,40}")) } ?: return
        val safeFields = fields.entries.mapNotNull { (key, value) ->
            if (!key.matches(Regex("[a-z_]{1,40}")) || !value.matches(Regex("[A-Za-z0-9._,:=/-]{1,2048}"))) {
                null
            } else {
                "$key=$value"
            }
        }
        Log.i(AppConfig.TAG, "FN_DIAG event=$safeEvent ${safeFields.joinToString(" ")}")
    }

    /** Stable local correlation token; raw profile and subscription IDs are never logged. */
    fun diagnosticId(value: String?): String {
        if (value.isNullOrBlank()) return "none"
        val digest = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8))
        return digest.take(8).joinToString("") { "%02x".format(Locale.US, it.toInt() and 0xff) }
    }

    fun d(tag: String = AppConfig.TAG, message: String) = log(Log.DEBUG, tag, message)
    fun i(tag: String = AppConfig.TAG, message: String) = log(Log.INFO, tag, message)
    fun w(tag: String = AppConfig.TAG, message: String) = log(Log.WARN, tag, message)
    fun e(tag: String = AppConfig.TAG, message: String) = log(Log.ERROR, tag, message)

    fun d(tag: String = AppConfig.TAG, message: String, throwable: Throwable) = log(Log.DEBUG, tag, message, throwable)
    fun i(tag: String = AppConfig.TAG, message: String, throwable: Throwable) = log(Log.INFO, tag, message, throwable)
    fun w(tag: String = AppConfig.TAG, message: String, throwable: Throwable) = log(Log.WARN, tag, message, throwable)
    fun e(tag: String = AppConfig.TAG, message: String, throwable: Throwable) = log(Log.ERROR, tag, message, throwable)
}

