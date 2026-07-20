Feature: Daemon and clients

  @slice1
  Scenario: Run the daemon
    Given a config with a listen address and DB path
    When I run sutra serve
    Then it opens the store and serves HTTP on that address

  @slice2
  Scenario: JSON API covers every operation
    Given the running daemon
    When any operation from the other epics is invoked over HTTP
    Then it accepts and returns JSON with one endpoint per operation

  @slice2
  Scenario: CLI is a thin client
    Given the daemon is running
    When I run a CLI command
    Then it calls exactly one API endpoint and holds no behavior the API doesn't expose
    Given the json flag
    When I run a CLI command
    Then it prints the API's raw JSON response

  @slice2
  Scenario: Authenticated LAN access
    Given a configured bearer token
    When a client sends a request with the correct token
    Then it succeeds
    Given a wrong or missing token
    When a client sends a request
    Then it is rejected with 401

  @slice2
  Scenario: Use clients from another machine
    Given a configured host set to the daemon's LAN address and a valid token
    When I run the CLI or TUI from another machine
    Then it operates against the daemon's data

  @slice8
  Scenario: Manage issues in the TUI
    Given the TUI is open
    When I create an issue
    Then it is added with the same rules as the create story and appears in the list
    Given an open issue
    When I edit its fields or change its status to closed
    Then the change persists and the ledger records it
    Given an issue
    When I create a child from it
    Then the new issue's parent_id is set to it
    Given an issue
    When I view it
    Then I can see its documents, comments, and linked transcripts
