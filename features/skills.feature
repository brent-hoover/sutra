Feature: Skills

  # Reusable agent skills stored in Sutra as a SKILL.md (name + description +
  # content), and a client-side install that writes a skill to the local
  # machine's skills directory as <slug>/SKILL.md so an agent can use it.

  @slice12
  Scenario: Create a skill
    Given a name and SKILL.md content
    When I create a skill
    Then the skill is stored with a generated id, a slug, and timestamps set

  @slice12
  Scenario: Slug is unique
    Given a skill already exists with a slug
    When I create another skill with the same slug
    Then it is rejected

  @slice12
  Scenario: List, view, and update a skill
    Given some skills exist
    When I list them, view one, and update its content
    Then the skill changes are reflected

  @slice12
  Scenario: Install a skill to the local skills directory
    Given a skill
    When I install it to a target directory
    Then its SKILL.md is written under that directory as <slug>/SKILL.md with the content

  @slice12
  Scenario: Delete a skill
    Given a skill
    When I delete the skill
    Then it is gone
