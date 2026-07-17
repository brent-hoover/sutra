Feature: Documents

  Scenario: Attach a document to an issue
    Given any issue
    When I attach a Document with a kind of problem, design, plan, or scenarios and markdown content
    Then it is stored linked to that issue with timestamps set
    Given empty content
    When I try to attach
    Then it is rejected

  Scenario: Read a document
    Given an issue with a document
    When I open the document by id
    Then its kind, title, and content are returned

  Scenario: List an issue's documents
    Given an issue with several documents
    When I list its documents
    Then all appear with their kind and title

  Scenario: Update a document
    Given an existing document
    When I update its content
    Then the new content is stored and updated_at advances

  Scenario: Remove a document
    Given an issue with a document
    When I remove it
    Then it no longer appears in the issue's document list
