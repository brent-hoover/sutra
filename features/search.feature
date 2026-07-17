Feature: Search

  @slice7
  Scenario: Full-text search across everything
    Given a known term appears in an issue, a document, and a transcript message
    When I search for that term
    Then hits from the issue, the document, and the message all appear

  @slice7
  Scenario: Soft-deleted content is excluded from search
    Given a known term appears in a live issue and in a soft-deleted issue
    When I search for that term
    Then the live issue appears and the soft-deleted issue is excluded

  @slice7
  Scenario: Message hits carry context
    Given a transcript whose middle message contains a known term
    When I search for that term
    Then the message hit includes its role, text, seq, owning transcript, and adjacent messages

  @slice7
  Scenario: Ranked results
    Given several matches of differing relevance for a term
    When I search for that term
    Then results are ordered by relevance and each hit shows its source kind

  @slice7
  Scenario: Scoped search
    Given a known term appears in both an issue and a document
    When I search for that term restricted to kind document
    Then only document hits are returned
