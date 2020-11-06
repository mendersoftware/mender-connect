// convert an unsafe pointer to a GDBusConnection structure
static GDBusConnection *to_gdbusconnection(void *ptr)
{
    return (GDBusConnection *)ptr;
}

// convert an unsafe pointer to a GDBusProxy structure
static GDBusProxy *to_gdbusproxy(void *ptr)
{
    return (GDBusProxy *)ptr;
}

// convert an unsafe pointer to a GVariant structure
static GVariant *to_gvariant(void *ptr)
{
    return (GVariant *)ptr;
}

// creates a new string from a GVariant
static gchar *string_from_g_variant(GVariant *value)
{
    gchar *str;
    g_variant_get(value, "(s)", &str);
    return str;
}

// creates a new boolean from a GVariant
static gboolean boolean_from_g_variant(GVariant *value)
{
    gboolean b;
    g_variant_get(value, "(b)", &b);
    return b;
}
