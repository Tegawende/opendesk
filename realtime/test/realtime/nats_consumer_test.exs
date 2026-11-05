defmodule Realtime.Nats.ConsumerTest do
  use ExUnit.Case, async: true

  alias Realtime.Nats.Consumer

  test "parses NATS_URL" do
    assert {:ok, %{host: "nats", port: 4222, tls: false}} = Consumer.settings("nats://nats", nil)

    assert {:ok, %{host: "10.0.0.1", port: 14_401}} =
             Consumer.settings("nats://10.0.0.1:14401", nil)

    assert {:ok, %{tls: true}} = Consumer.settings("tls://nats.example:4222", nil)

    assert {:ok, %{token: token}} = Consumer.settings("nats://nats:4222", "s3cret")
    assert token.() == "s3cret"

    for bad <- ["", "http://nats:4222", "nats://", "nats:4222"] do
      assert Consumer.settings(bad, nil) == {:error, :invalid_nats_url}, bad
    end
  end

  test "parses an IPv6 literal in NATS_URL" do
    assert {:ok, %{host: "fd12::1", port: 4222}} = Consumer.settings("nats://[fd12::1]:4222", nil)
  end

  test "connects over IPv4 when the host has an IPv4 address, else over IPv6" do
    assert Consumer.tcp_opts("127.0.0.1") == [:binary]
    assert Consumer.tcp_opts("::1") == [:binary, :inet6]
    assert Consumer.tcp_opts("fd12:12c0:9cb1:0:b000:143:315b:924b") == [:binary, :inet6]
    # An unresolvable host stays on IPv4, so the error reported is the lookup's.
    assert Consumer.tcp_opts("nats.invalid") == [:binary]
  end

  test "reaches a listener on an IPv6-only address" do
    v6_only = [:binary, :inet6, ip: {0, 0, 0, 0, 0, 0, 0, 1}, ipv6_v6only: true]

    case :gen_tcp.listen(0, v6_only) do
      {:ok, listener} ->
        {:ok, port} = :inet.port(listener)
        assert {:ok, sock} = :gen_tcp.connect(~c"::1", port, Consumer.tcp_opts("::1"), 1_000)
        :gen_tcp.close(sock)
        :gen_tcp.close(listener)

      {:error, _} ->
        # No IPv6 loopback on this machine.
        :ok
    end
  end

  test "backoff grows exponentially to 30 s" do
    assert Consumer.backoff(0) in 500..625
    assert Consumer.backoff(1) in 1_000..1_250
    assert Consumer.backoff(20) in 30_000..37_500
  end

  test "stays up and keeps retrying while NATS is unreachable" do
    # Port 1 refuses connections.
    pid = start_supervised!({Consumer, nats: [url: "nats://127.0.0.1:1"]}, id: :down)
    Process.sleep(100)
    assert Process.alive?(pid)
    refute :sys.get_state(pid).conn
    assert :sys.get_state(pid).attempt >= 1
  end
end

defmodule Realtime.Nats.ConsumerLiveTest do
  # Runs against a real NATS server: NATS_TEST_URL=nats://127.0.0.1:14401 mix test --only nats
  use ExUnit.Case, async: false

  @moduletag :nats

  alias Phoenix.Socket.Broadcast
  alias Realtime.Nats.Consumer
  alias Realtime.Test.Tokens
  alias RealtimeWeb.Endpoint

  setup do
    url = System.get_env("NATS_TEST_URL") || "nats://127.0.0.1:4222"
    token = System.get_env("NATS_TEST_TOKEN")
    start_supervised!({Consumer, name: :live_consumer, nats: [url: url, token: token]})
    {:ok, settings} = Consumer.settings(url, token)
    {:ok, pub} = Gnat.start_link(Map.put(settings, :tcp_opts, Consumer.tcp_opts(settings.host)))
    wait_connected(:live_consumer, 50)
    %{pub: pub}
  end

  defp wait_connected(_name, 0), do: flunk("consumer did not connect")

  defp wait_connected(name, n) do
    if :sys.get_state(name).conn, do: :ok, else: Process.sleep(100) && wait_connected(name, n - 1)
  end

  test "an envelope published to od.events.<wid> is broadcast", %{pub: pub} do
    wid = Tokens.uuid()
    :ok = Endpoint.subscribe("workspace:" <> wid)
    # The subscription is set up asynchronously on the server; give it a moment.
    Process.sleep(100)

    env = %{
      "id" => Tokens.uuid(),
      "seq" => 1,
      "workspace_id" => wid,
      "type" => "ticket.created",
      "visibility" => "agents",
      "topics" => ["workspace:" <> wid],
      "data" => %{},
      "occurred_at" => "2026-10-11T09:00:00Z"
    }

    :ok = Gnat.pub(pub, "od.events." <> wid, Jason.encode!(env))
    assert_receive %Broadcast{event: "event", payload: %{"seq" => 1}}, 2_000
  end

  test "od.control.disconnect is broadcast to the socket id", %{pub: pub} do
    uid = Tokens.uuid()
    :ok = Endpoint.subscribe("user_socket:" <> uid)
    Process.sleep(100)
    :ok = Gnat.pub(pub, "od.control.disconnect", Jason.encode!(%{"user_id" => uid}))
    assert_receive %Broadcast{event: "disconnect"}, 2_000
  end
end
