module Dapp
  # Image
  module Image
    # Stage
    class Stage < Docker
      include Argument

      def initialize(name:, project:, built_id: nil, from: nil)
        @container_name = "#{name[/[[^:].]*/]}.#{SecureRandom.hex(4)}"
        @built_id = built_id

        @bash_commands          = []
        @options                = {}
        @change_options         = {}
        @service_change_options = {}

        super(name: name, project: project, from: from)
      end

      def built_id
        @built_id ||= id
      end

      def build!
        run!
        @built_id = commit!
      ensure
        project.shellout("docker rm #{container_name}")
      end

      def export!(name)
        image = self.class.new(name: name, project: project, built_id: built_id)
        image.tag!
        image.push!
        image.untag!
      end

      def import!(name)
        image = self.class.new(name: name, project: project)
        image.pull!
        @built_id = image.built_id
        tag!
        image.untag!
      end

      def tag!
        project.log_warning(desc: { code: :another_image_already_tagged, context: 'warning' }) if !(existed_id = id).nil? && built_id != existed_id
        project.shellout!("docker tag #{built_id} #{name}")
        cache_reset
      end

      def labels
        raise Error::Build, code: :image_not_exist, data: { name: name } if built_id.nil?
        self.class.image_config_option(image_id: built_id, option: 'labels')
      end

      protected

      attr_reader :container_name

      def run!
        raise Error::Build, code: :built_id_not_defined if from.built_id.nil?
        project.shellout!("docker run #{prepared_options} #{from.built_id} -ec '#{prepared_bash_command}'", log_verbose: true)
      rescue Error::Shellout => _e
        raise unless project.introspect_error? || project.introspect_before_error?
        built_id = project.introspect_error? ? commit! : from.built_id
        raise Exception::IntrospectImage, data: { built_id: built_id, options: prepared_options, rmi: project.introspect_error? }
      end

      def commit!
        project.shellout!("docker commit #{prepared_change} #{container_name}").stdout.strip
      end
    end # Stage
  end # Image
end # Dapp
