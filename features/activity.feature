Feature: Activity feed (context for humans)

  # "When I come back Monday, show me what we were working on Friday."
  # A reverse-chronological feed across issues and captured Claude transcripts,
  # scoped to a time window, that auto-ingests recent sessions first.

  @slice9
  Scenario: Catch up on recent activity
    Given issues that changed and a transcript captured within the window
    When I run sutra activity for that window
    Then I see a reverse-chronological feed of the issue changes and the transcript

  @slice9
  Scenario: Auto-ingest surfaces an un-captured session
    Given a Claude session file on disk within the window that has not been ingested
    When I run sutra activity for that window
    Then the session is ingested and appears in the feed

  @slice9
  Scenario: The window excludes older activity
    Given activity older than the window and activity within it
    When I run sutra activity for that window
    Then only the activity within the window is shown
