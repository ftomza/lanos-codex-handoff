package io.github.ftomza.lanos

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class FuelControlRequestTest {
    @Test fun acceptsOnlyTypedLocalOperations() {
        val reset = """{"type":"fuel","id":1,"operation":"reset","body":{"expected_revision":1,"default_factor":false}}"""
        assertEquals(FuelControlRequest(1, 1, FuelAction.Reset(false)), FuelControlRequest.decode(reset))
        for (bad in listOf(
            reset.replace("reset", "ecu"),
            reset.replace("false", "\"false\""),
            reset.replace("false", "false,\"command\":\"04\""),
            reset.replace("revision\":1", "revision\":1.5"),
            reset + "garbage",
            reset + " ".repeat(1024),
        )) assertNull(bad, FuelControlRequest.decode(bad))
    }
}
