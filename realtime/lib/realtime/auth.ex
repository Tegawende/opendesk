defmodule Realtime.Auth do
  @moduledoc """
  Socket token verification (`contracts/tokens.md`).

  Tokens are HS256 JWTs signed with `AUTH_SECRET`. Only HS256 is accepted, `exp`
  is required and must be in the future, and the `purpose` claim must be `ws`
  (agents and API keys) or `widget_ws` (chat widget contacts). Nothing about the
  token itself is ever logged or returned.
  """

  alias Realtime.Ids

  @min_secret_bytes 32
  # Room for a `teams` claim of several dozen private teams, within the
  # request line Bandit accepts (the token travels in the query string).
  @max_token_bytes 8192
  @max_teams 200
  @roles ~w(owner admin agent viewer)
  @kinds %{"user" => :user, "api_key" => :api_key}

  @type identity :: %{
          kind: :user | :api_key | :contact,
          sub: String.t(),
          wid: String.t(),
          role: String.t() | nil,
          name: String.t() | nil,
          avatar: String.t() | nil,
          teams: [String.t()],
          brand: String.t() | nil,
          sid: String.t() | nil,
          issued_ms: integer()
        }

  @doc "The smallest `AUTH_SECRET` the gateway boots with."
  def min_secret_bytes, do: @min_secret_bytes

  @doc "Checks a secret is usable for HS256."
  @spec validate_secret(term()) :: :ok | {:error, :secret_too_short}
  def validate_secret(secret) when is_binary(secret) and byte_size(secret) >= @min_secret_bytes,
    do: :ok

  def validate_secret(_), do: {:error, :secret_too_short}

  @doc """
  Verifies `token` and returns the identity it carries.

  Errors are coarse atoms (safe to log and count): `:missing_token`,
  `:invalid_token`, `:expired`, `:wrong_purpose`, `:invalid_claims`.
  """
  @spec verify(term(), keyword()) :: {:ok, identity()} | {:error, atom()}
  def verify(token, opts \\ [])
  def verify(token, _opts) when token in [nil, ""], do: {:error, :missing_token}

  def verify(token, opts) when is_binary(token) and byte_size(token) <= @max_token_bytes do
    secret =
      Keyword.get_lazy(opts, :secret, fn -> Application.fetch_env!(:realtime, :auth_secret) end)

    now = Keyword.get_lazy(opts, :now, fn -> System.system_time(:second) end)

    with :ok <- validate_secret(secret),
         {:ok, claims} <- verify_signature(token, secret),
         :ok <- check_time(claims, now) do
      identity(claims)
    else
      {:error, :secret_too_short} -> {:error, :invalid_token}
      error -> error
    end
  end

  def verify(_token, _opts), do: {:error, :invalid_token}

  # verify_strict refuses any alg outside the list, which rules out "none",
  # HS384/HS512 and every asymmetric alg (and so key-confusion attacks).
  defp verify_signature(token, secret) do
    case JOSE.JWT.verify_strict(JOSE.JWK.from_oct(secret), ["HS256"], token) do
      {true, %JOSE.JWT{fields: fields}, _jws} ->
        {:ok, fields}

      _ ->
        {:error, :invalid_token}
    end
  rescue
    _ -> {:error, :invalid_token}
  catch
    _, _ -> {:error, :invalid_token}
  end

  defp check_time(%{"exp" => exp} = claims, now) when is_integer(exp) do
    cond do
      exp <= now -> {:error, :expired}
      not_yet_valid?(claims, now) -> {:error, :invalid_token}
      true -> :ok
    end
  end

  defp check_time(_claims, _now), do: {:error, :invalid_claims}

  defp not_yet_valid?(%{"nbf" => nbf}, now) when is_integer(nbf), do: nbf > now
  defp not_yet_valid?(%{"nbf" => _}, _now), do: true
  defp not_yet_valid?(_claims, _now), do: false

  defp identity(%{"purpose" => "ws"} = c) do
    with {:ok, sub} <- uuid(c["sub"]),
         {:ok, wid} <- uuid(c["wid"]),
         {:ok, kind} <- Map.fetch(@kinds, c["kind"]),
         role when role in @roles <- c["role"],
         {:ok, teams} <- teams(c["teams"]),
         {:ok, sid} <- optional_uuid(c["sid"]) do
      {:ok,
       %{
         kind: kind,
         sub: sub,
         wid: wid,
         role: role,
         name: string(c["name"], 200),
         avatar: string(c["avatar"], 2048),
         teams: teams,
         brand: nil,
         sid: sid,
         issued_ms: issued_ms(c)
       }}
    else
      _ -> {:error, :invalid_claims}
    end
  end

  # `bid` binds a widget socket to the brand of the widget its token was issued for: it
  # receives only that brand's chats (`RealtimeWeb.ContactChannel`).
  defp identity(%{"purpose" => "widget_ws"} = c) do
    with {:ok, sub} <- uuid(c["sub"]),
         {:ok, wid} <- uuid(c["wid"]),
         {:ok, brand} <- optional_uuid(c["bid"]) do
      {:ok,
       %{
         kind: :contact,
         sub: sub,
         wid: wid,
         role: nil,
         name: nil,
         avatar: nil,
         teams: [],
         brand: brand,
         sid: nil,
         issued_ms: issued_ms(c)
       }}
    else
      _ -> {:error, :invalid_claims}
    end
  end

  defp identity(_claims), do: {:error, :wrong_purpose}

  # The issue time in ms that revocations are compared with (`Realtime.Revocations`):
  # `iat_ms`, else whole-second `iat`. A token with neither counts as issued at 0, before
  # any revocation.
  defp issued_ms(%{"iat_ms" => ms}) when is_integer(ms) and ms > 0, do: ms
  defp issued_ms(%{"iat" => iat}) when is_integer(iat) and iat > 0, do: iat * 1000
  defp issued_ms(_claims), do: 0

  defp teams(nil), do: {:ok, []}

  defp teams(list) when is_list(list) and length(list) <= @max_teams do
    if Enum.all?(list, &Ids.uuid?/1),
      do: {:ok, list |> Enum.map(&Ids.normalize/1) |> Enum.uniq()},
      else: :error
  end

  defp teams(_), do: :error

  defp optional_uuid(nil), do: {:ok, nil}
  defp optional_uuid(value), do: uuid(value)

  defp uuid(value) do
    if Ids.uuid?(value), do: {:ok, Ids.normalize(value)}, else: :error
  end

  defp string(value, max) when is_binary(value) and byte_size(value) <= max, do: value
  defp string(_value, _max), do: nil
end
