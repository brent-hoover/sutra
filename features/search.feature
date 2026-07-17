Feature: Search

  Scenario: Full-text search across everything
    Given a known term present in an Issue subject or body, a Document content, and a Message text
    When I search for that term
    Then hits from all three appear
    Given soft-deleted issues
    When I search
    Then their content is excluded

  Scenario: Message hits carry context
    Given a matching Message
    When it is returned
    Then it includes its role, text, seq, and its owning Transcript and linked Issue if any
    Given a matching Message
    When it is returned again
    Then it includes a window of adjacent messages for context

  Scenario: Ranked results
    Given multiple matches
    When I search
    Then results are ranked by relevance and each shows its source kind of issue, document, or message

  Scenario: Scoped search
    Given a scope such as kind=document or a single issue
    When I search within it
    Then only matches in that scope are returned
