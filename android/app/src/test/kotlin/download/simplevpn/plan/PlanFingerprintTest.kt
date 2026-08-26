package download.simplevpn.plan

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

/**
 * What makes one plan different from another.
 *
 * This decides whether a plan gets a fresh chance after failing, and getting it
 * wrong made the rollback inert: the server issues a new sequence number on
 * every request, so treating "new number" as "new plan" meant every retry reset
 * the failure count and the same broken plan was tried for ever. The mechanism
 * existed, looked right, and could never fire.
 *
 * Found by walking through the live test before running it, which is the only
 * reason it is not still there.
 */
class PlanFingerprintTest {

    private fun plan(
        seq: Long = 1,
        host: String = "10.0.0.1",
        directDomains: String = """["domain:nalog.ru"]""",
        russiaDirect: Boolean = true,
    ) = ConnectionPlan.parse(
        JSONObject(
            """
            {"v":1,"seq":$seq,"expires_at":"2030-01-01T00:00:00Z",
             "primary":{"alias":"n-1","host":"$host","port":443,
               "transport":{"kind":"vless-ws-tls","params":{
                 "credential_uuid":"00000000-0000-4000-8000-000000000000",
                 "path":"/x","server_name":"example.invalid"}}},
             "reserves":[],
             "routing":{"direct_domains":$directDomains,"russia_direct":$russiaDirect},
             "policy":{"probe_interval_s":60}}
            """.trimIndent(),
        ),
    )!!

    @Test
    fun `the same instructions with a new number are the same plan`() {
        // The failure this exists for. Without it the rollback never fires.
        assertEquals(plan(seq = 1).fingerprint, plan(seq = 999).fingerprint)
    }

    @Test
    fun `a different node is a different plan`() {
        assertNotEquals(plan(host = "10.0.0.1").fingerprint, plan(host = "10.0.0.2").fingerprint)
    }

    @Test
    fun `different routing is a different plan`() {
        // A wrong routing rule is exactly the kind of mistake this stage exists
        // to survive, so correcting one must give the plan a fresh chance.
        assertNotEquals(
            plan(directDomains = """["domain:nalog.ru"]""").fingerprint,
            plan(directDomains = """["domain:nalog.ru","domain:vtb.ru"]""").fingerprint,
        )
    }

    @Test
    fun `switching off the Russian rule is a different plan`() {
        assertNotEquals(
            plan(russiaDirect = true).fingerprint,
            plan(russiaDirect = false).fingerprint,
        )
    }
}
