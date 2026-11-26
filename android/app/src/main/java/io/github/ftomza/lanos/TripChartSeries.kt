package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONArray
import org.json.JSONObject

private data class ChartPoint(val tick: Long, val value: Double, val segment: Int)

internal fun chartPoints(samples: List<HistorySample>, index: Int, start: Instant, duration: Long): JSONArray {
    val output = JSONArray()
    val reduce = samples.size > 1024
    var segment = 0
    var lastSegment = -1
    var bucket = -1
    var first: ChartPoint? = null
    var min: ChartPoint? = null
    var max: ChartPoint? = null
    var last: ChartPoint? = null

    fun append(point: ChartPoint) {
        output.put(JSONObject().put("at", wall(start, point.tick)).put("value", point.value)
            .put("break", point.segment != lastSegment))
        lastSegment = point.segment
    }

    fun emitBucket() {
        listOfNotNull(first, min, max, last).distinct().sortedBy(ChartPoint::tick).forEach(::append)
        first = null
        min = null
        max = null
        last = null
    }

    val flowSeries = tripCharts[index] == "flow_lph"
    val fuelSeries = flowSeries || tripCharts[index] == "liters" || tripCharts[index] == "average_l100"
    for (sample in samples) {
        val value = sample.values[index]
        if (sample.breakAll || (fuelSeries && sample.breakCounters) ||
            (flowSeries && sample.breakFlow) || !value.isFinite()) segment++
        if (!value.isFinite()) continue
        val point = ChartPoint(sample.tick, value, segment)
        if (!reduce) {
            append(point)
            continue
        }
        val nextBucket = (sample.tick * 256 / maxOf(1, duration)).toInt().coerceIn(0, 255)
        if (bucket != nextBucket) {
            emitBucket()
            bucket = nextBucket
        }
        if (first == null) first = point
        if (min == null || value < checkNotNull(min).value) min = point
        if (max == null || value > checkNotNull(max).value) max = point
        last = point
    }
    emitBucket()
    return output
}
