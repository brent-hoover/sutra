Feature: Projects

  # A Project is a first-class container for work in one repository (1 project =
  # 1 repo). Issues and threads can be scoped to a project, and captured Claude
  # transcripts auto-associate to the project whose repo path matches the
  # session's working directory.

  @slice10
  Scenario: Create a project
    Given a name and a repo path
    When I create a project
    Then it is stored with a generated id, a slug, and timestamps set

  @slice10
  Scenario: Repo path is unique
    Given a project already exists for a repo path
    When I create another project for the same repo path
    Then it is rejected

  @slice10
  Scenario: List, view, and update a project
    Given some projects exist
    When I list them, view one, and update its name
    Then the changes are reflected

  @slice10
  Scenario: Deleting a project detaches its references
    Given a project with an issue scoped to it
    When I delete the project
    Then the project is gone and the issue's project is cleared

  @slice10
  Scenario: Scope an issue to a project
    Given a project
    When I create an issue in that project
    Then the issue lists that project and appears when filtering by it

  @slice10
  Scenario: A transcript auto-associates to its project by repo path
    Given a project whose repo path matches a session's working directory
    When the session transcript is ingested
    Then the transcript is associated with that project
