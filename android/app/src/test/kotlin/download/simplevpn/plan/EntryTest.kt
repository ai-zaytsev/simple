package download.simplevpn.plan

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The ways in, and the order they are tried.
 *
 * Worth testing without a device because the situation these exist for cannot
 * be produced on demand: a domain blocked, a resolver poisoned, an address
 * unreachable. The behaviour has to be right the first time it happens on
 * somebody's phone, and that is not the moment to find out that every client
 * tries the same entry first.
 */
class EntryTest {

    private fun entry(json: String) = Entry.parse(JSONObject(json))

    @Test
    fun `the build knows more than one way in`() {
        // One is a single point of recovery, which is the thing this stage
        // forbids. A seed with one entry is a build that dies with one domain.
        assertTrue(Entry.SEED.size >= 2)
        assertTrue(Entry.SEED.map { it.kind }.toSet().size >= 2)
    }

    @Test
    fun `the seed carries a way in that needs no resolver`() {
        // The entry that survives a blocked or poisoned resolver. Without it in
        // the build, a first installation during a DNS block has nothing.
        val address = Entry.SEED.first { it.kind == Entry.Kind.ADDRESS }
        assertTrue("an address entry must carry the name to expect", address.serverName.isNotBlank())
    }

    @Test
    fun `a name entry is read`() {
        val parsed = entry("""{"kind":"https-direct","host":"a.example","weight":100}""")!!
        assertEquals(Entry.Kind.NAME, parsed.kind)
        assertEquals("https://a.example", parsed.baseUrl)
        assertEquals("a.example", parsed.expectedName)
    }

    @Test
    fun `an address entry expects the name, not the address`() {
        val parsed = entry(
            """{"kind":"https-ip","host":"1.2.3.4","server_name":"a.example","weight":80}""",
        )!!
        assertEquals("https://1.2.3.4", parsed.baseUrl)
        // The certificate must still be checked, and against the name. An
        // address entry that accepted any certificate would be a way in for
        // whoever is doing the blocking.
        assertEquals("a.example", parsed.expectedName)
    }

    @Test
    fun `an edge entry keeps its path`() {
        val parsed = entry(
            """{"kind":"https-edge","host":"b.example","path_prefix":"/abc123","weight":60}""",
        )!!
        assertEquals("https://b.example/abc123", parsed.baseUrl)
    }

    @Test
    fun `a port other than the usual one is kept`() {
        val parsed = entry("""{"kind":"https-direct","host":"a.example","port":8443}""")!!
        assertEquals("https://a.example:8443", parsed.baseUrl)
    }

    @Test
    fun `a kind this build does not know is skipped, not fatal`() {
        // Ignoring it is what lets the server introduce a kind before every
        // installation understands it. Refusing the whole descriptor would
        // strand exactly the clients being migrated.
        assertNull(entry("""{"kind":"carrier-pigeon","host":"a.example"}"""))
        assertNotNull(entry("""{"kind":"https-direct","host":"a.example"}"""))
    }

    @Test
    fun `an entry with no host is not an entry`() {
        assertNull(entry("""{"kind":"https-direct","host":""}"""))
    }

    @Test
    fun `every entry is tried, none is lost`() {
        val entries = listOf(
            Entry(Entry.Kind.NAME, "a", 443, "", "", 100),
            Entry(Entry.Kind.ADDRESS, "b", 443, "a", "", 50),
            Entry(Entry.Kind.EDGE, "c", 443, "", "/x", 10),
        )
        val ordered = Entry.order(entries) { bound -> bound - 1 }
        assertEquals(entries.size, ordered.size)
        assertEquals(entries.toSet(), ordered.toSet())
    }

    @Test
    fun `the order is not always the same`() {
        // One order for every client is a signature, and it puts the whole
        // installed base on whichever entry happens to be first. Two different
        // draws must be able to produce two different orders.
        val entries = listOf(
            Entry(Entry.Kind.NAME, "a", 443, "", "", 100),
            Entry(Entry.Kind.ADDRESS, "b", 443, "a", "", 100),
        )
        val first = Entry.order(entries) { 0 }.map { it.host }
        val second = Entry.order(entries) { bound -> bound - 1 }.map { it.host }
        assertTrue("a different draw must be able to give a different order", first != second)
    }

    @Test
    fun `an empty list orders to nothing rather than failing`() {
        assertTrue(Entry.order(emptyList()) { 0 }.isEmpty())
    }
}
