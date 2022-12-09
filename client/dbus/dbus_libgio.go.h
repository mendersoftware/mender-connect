// Copyright 2022 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

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

// convert an unsafe pointer to a GMainLoop structure
static GMainLoop *to_gmainloop(void *ptr)
{
    return (GMainLoop *)ptr;
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

// creates a new string from a GVariant containing two strings
static gchar *first_string_from_g_variant(GVariant *value)
{
    gchar *str1;
    gchar *str2;
    g_variant_get(value, "(ss)", &str1, &str2);
    return str1;
}

// creates a new string from a GVariant containing two strings
static gchar *second_string_from_g_variant(GVariant *value)
{
    gchar *str1;
    gchar *str2;
    g_variant_get(value, "(ss)", &str1, &str2);
    return str2;
}

// creates a new boolean from a GVariant
static gboolean boolean_from_g_variant(GVariant *value)
{
    gboolean b;
    g_variant_get(value, "(b)", &b);
    return b;
}

// exported by golang, see dbus_libgio.go
void handle_on_signal_callback(
    GDBusProxy *proxy,
    gchar *sender_name,
    gchar *signal_name,
    GVariant *parameters,
    gpointer user_data);

// callback registered via g_signal_connect
static void on_signal(
    GDBusProxy *proxy,
    gchar *sender_name,
    gchar *signal_name,
    GVariant *parameters,
    gpointer user_data)
{
    handle_on_signal_callback(
        proxy, sender_name, signal_name, parameters, user_data);
}

// calls g_signal_connect on a proxy instance
static void g_signal_connect_on_proxy(GDBusProxy *proxy)
{
    g_signal_connect(proxy, "g-signal", G_CALLBACK(on_signal), NULL);
}
