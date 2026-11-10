package com.v2ray.ang.handler

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.v2ray.ang.AngApplication
import com.v2ray.ang.AppConfig
import com.v2ray.ang.core.CoreNativeManager
import com.v2ray.ang.service.RealPingWorkerService
import com.v2ray.ang.util.LogUtil
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import java.util.concurrent.TimeUnit

/** Schedules bounded, cancellable local probes after the dedicated subscription changes. */
object FreeNodeProbeScheduler {
    private const val WORK_NAME = "free-node-probe"

    fun schedule(context: Context = AngApplication.application) {
        val request = OneTimeWorkRequestBuilder<FreeNodeProbeTask>()
            .setConstraints(
                Constraints.Builder()
                    .setRequiredNetworkType(NetworkType.CONNECTED)
                    .setRequiresBatteryNotLow(true)
                    .build(),
            )
            .setBackoffCriteria(BackoffPolicy.LINEAR, 15, TimeUnit.MINUTES)
            .addTag(WORK_NAME)
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(WORK_NAME, ExistingWorkPolicy.REPLACE, request)
    }

    fun cancel(context: Context = AngApplication.application) {
        WorkManager.getInstance(context).cancelUniqueWork(WORK_NAME)
    }

    class FreeNodeProbeTask(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
        override suspend fun doWork(): Result {
            if (!FreeNodeManager.isEnabled()) return Result.success()
            val candidates = FreeNodeManager.probePlan()
            if (candidates.isEmpty()) return Result.success()

            return try {
                CoreNativeManager.initCoreEnv(applicationContext)
                candidates.forEach { (fingerprint, guid) ->
                    currentCoroutineContext().ensureActive()
                    if (isStopped) return Result.success()
                    val delay = RealPingWorkerService.measure(applicationContext, guid)
                    if (!isStopped) FreeNodeManager.recordProbe(fingerprint, delay)
                }
                Result.success()
            } catch (error: Exception) {
                LogUtil.e(AppConfig.TAG, "Free-node probe batch failed", error)
                Result.retry()
            }
        }
    }
}
