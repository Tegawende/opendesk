defmodule Realtime.Nats.Consumer do
  @moduledoc """
  Owns the NATS connection and the subscriptions, and feeds every message to
  `Realtime.Fanout` in arrival order.

  The connection is linked and exits are trapped, so a NATS outage never takes
  the gateway down: the process reconnects with exponential backoff (500 ms
  doubling to 30 s, with jitter) and subscribes again. Messages are handled in
  this process, one at a time, which keeps per-workspace `seq` order intact.

  There is no queue group on purpose: every node must see every message (see
  `Realtime.Fanout`).
  """

  use GenServer

  require Logger

  alias Realtime.Fanout
  alias Realtime.Metrics

  @base_backoff 500
  @max_backoff 30_000

  def start_link(opts),
    do: GenServer.start_link(__MODULE__, opts, name: Keyword.get(opts, :name, __MODULE__))

  @doc "True while a NATS connection is up."
  def connected? do
    case Process.whereis(__MODULE__) do
      nil -> false
      pid -> GenServer.call(pid, :connected?, 1_000)
    end
  catch
    :exit, _ -> false
  end

  @doc "Parses `NATS_URL` (`nats://host:port` or `tls://host:port`) into Gnat settings."
  @spec settings(String.t(), String.t() | nil) :: {:ok, map()} | {:error, :invalid_nats_url}
  def settings(url, token) do
    case URI.parse(url) do
      %URI{scheme: scheme, host: host, port: port}
      when scheme in ["nats", "tls"] and is_binary(host) and host != "" ->
        base = %{
          host: host,
          port: port || 4222,
          tls: scheme == "tls",
          name: "opendesk-realtime",
          connection_timeout: 5_000
        }

        # A function rather than the value keeps the token out of crash reports.
        {:ok, if(token, do: Map.put(base, :token, fn -> token end), else: base)}

      _ ->
        {:error, :invalid_nats_url}
    end
  end

  @impl true
  def init(opts) do
    Process.flag(:trap_exit, true)
    nats = Keyword.get(opts, :nats, Application.get_env(:realtime, :nats, []))

    case settings(Keyword.get(nats, :url, ""), Keyword.get(nats, :token)) do
      {:ok, settings} ->
        send(self(), :connect)
        {:ok, %{settings: settings, conn: nil, attempt: 0}}

      {:error, reason} ->
        {:stop, reason}
    end
  end

  @impl true
  def handle_call(:connected?, _from, state), do: {:reply, state.conn != nil, state}

  @impl true
  def handle_info(:connect, state) do
    %{host: host, port: port} = state.settings
    settings = Map.put(state.settings, :tcp_opts, tcp_opts(host))

    with {:ok, conn} <- Gnat.start_link(settings),
         :ok <- subscribe(conn) do
      Logger.info("NATS connected to #{host}:#{port}")
      {:noreply, %{state | conn: conn, attempt: 0}}
    else
      error ->
        Metrics.incr(:nats_connect_failed)
        delay = backoff(state.attempt)

        Logger.warning("NATS unavailable at #{host}:#{port}, retrying in #{delay} ms",
          reason: reason(error)
        )

        Process.send_after(self(), :connect, delay)
        {:noreply, %{state | attempt: state.attempt + 1}}
    end
  end

  def handle_info({:msg, %{topic: subject, body: body}}, state) do
    try do
      Fanout.handle(subject, body)
    rescue
      e ->
        Metrics.incr(:fanout_crashed)
        Realtime.PostHog.capture_exception(e, __STACKTRACE__)
        Logger.error("fan-out failed", reason: inspect(e.__struct__))
    end

    {:noreply, state}
  end

  def handle_info({:EXIT, conn, _reason}, %{conn: conn} = state) do
    Metrics.incr(:nats_disconnected)
    Logger.warning("NATS connection lost, reconnecting")
    Process.send_after(self(), :connect, backoff(0))
    {:noreply, %{state | conn: nil, attempt: 1}}
  end

  def handle_info({:EXIT, _pid, _reason}, state), do: {:noreply, state}
  def handle_info(_other, state), do: {:noreply, state}

  @impl true
  def terminate(_reason, %{conn: conn}) when is_pid(conn), do: Gnat.stop(conn)
  def terminate(_reason, _state), do: :ok

  defp subscribe(conn) do
    Enum.reduce_while(Fanout.subjects(), :ok, fn subject, :ok ->
      case Gnat.sub(conn, self(), subject) do
        {:ok, _sid} ->
          {:cont, :ok}

        error ->
          Gnat.stop(conn)
          {:halt, error}
      end
    end)
  end

  @doc false
  def backoff(attempt) do
    exp = min(@max_backoff, @base_backoff * Integer.pow(2, min(attempt, 10)))
    exp + :rand.uniform(div(exp, 4) + 1) - 1
  end

  @doc """
  TCP options for reaching `host`: IPv4 when it has an IPv4 address, else IPv6
  when it has an IPv6 one, since `:gen_tcp` looks up one family only. Some
  private networks are IPv6 only. Looked up on every connect, as the host's
  addresses can change between attempts.
  """
  @spec tcp_opts(String.t()) :: [atom()]
  def tcp_opts(host) do
    name = String.to_charlist(host)

    with {:error, _} <- :inet.getaddr(name, :inet),
         {:ok, _} <- :inet.getaddr(name, :inet6) do
      [:binary, :inet6]
    else
      _ -> [:binary]
    end
  end

  defp reason({:error, %{__exception__: true} = e}), do: inspect(e.__struct__)
  defp reason({:error, reason}) when is_atom(reason), do: reason
  defp reason({:error, {reason, _}}) when is_atom(reason), do: reason
  defp reason(_), do: :error
end
