Feature: Working through a plan

  @slice14
  Scenario: Enumerate a plan's tracers in order
    Given an approved plan issue with 3 tracer children chained in order, all open
    When I list issues filtered by the plan issue as parent
    Then exactly the 3 tracer children are returned in run order, each with its status
    Given an issue with no children
    When I list issues filtered by that issue as parent
    Then no issues are returned
    Given a status filter of done
    When I list a plan's children with that filter
    Then it is rejected as an invalid filter

  @slice14
  Scenario: Completing a tracer is reflected on the next traversal
    Given an approved plan whose first tracer is open and blocks the second
    When I close the first tracer and list the plan's children again
    Then the first tracer is returned with status closed
    And the second tracer is returned with status open
    And the block edge from the first tracer to the second still exists
